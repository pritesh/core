// Copyright (c) 2016 Pani Networks
// All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// Authentication-related code.

package common

import (
	"crypto/rsa"
	"flag"
	"fmt"
	"net/http"
	"syscall"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/context"
	log "github.com/romana/rlog"
	cli "github.com/spf13/cobra"
	config "github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	RoleAdmin   = "admin"
	RoleService = "service"
	RoleTenant  = "tenant"
)

// DefaultAdminUser is a dummy user having admin role. It is used when
// authentication is off.
var DefaultAdminUser User

func init() {
	DefaultAdminUser = User{Roles: []Role{Role{Name: RoleAdmin}}}
}

// AuthTokenMessage is returned by Root service upon a client's authentication.
// The token is generated by the root server depending on the information looked
// up on the user (roles and attributes) and the public key is the root server's
// public key to verify the token (which is signed by root's private key).
type AuthTokenMessage struct {
	Token     string `json:"token"`
	PublicKey []byte `json:"public_key"`
}

type Role struct {
	Name string `json:"name"`
	Id   int    `sql:"AUTO_INCREMENT"`
}

// User has multiple roles and multiple attributes.
type User struct {
	UserId             int `sql:"AUTO_INCREMENT" json:"user_id" gorm:"primary_key"`
	jwt.StandardClaims `json:"claims"`
	Username           string      `json:"username,omitempty"`
	Password           string      `json:"password,omitempty"`
	Roles              []Role      `gorm:"many2many:user_roles;ForeignKey:user_id"`
	Attributes         []Attribute `gorm:"many2many:user_attributes;ForeignKey:user_id"`
}

// An Attribute of a user is something that is used in the ABAC
// part of our AuthZ scheme. Not every role would be checked for
// atributes. For now, only if the user has a role of tenant,
// it's attribute for key "tenant" is checked against the tenant ID
// by tenant service.
type Attribute struct {
	AttributeKey   string `json:"attribute_key"`
	AttributeValue string `json:"attribute_value"`
	Id             int    `sql:"AUTO_INCREMENT"`
}

// Represents the type of credential (e.g., certificate,
// username-password, etc.)
type CredentialType string

const (
	CredentialUsernamePassword = "userPass"
	CredentialNone             = "none"

	UsernameKey = "ROMANA_USERNAME"
	PasswordKey = "ROMANA_PASSWORD"
)

// Container for various credentials. Currently containing Username/Password
// but keys, certificates, etc. can be used in the future.
type Credential struct {
	Type    CredentialType
	cmd     *cli.Command
	flagSet *flag.FlagSet
	// In case of usage with Cobra (https://github.com/spf13/cobra/)
	// no need to check
	assumeFlagParsed bool
	Username         string
	Password         string
	userFlag         string
	passFlag         string
}

func (c *Credential) String() string {
	switch c.Type {
	case CredentialUsernamePassword:
		return fmt.Sprintf("Type: %s, user: %s", c.Type, c.Username)
	default:
		return "None"
	}
}

func NewCredentialCobra(cmd *cli.Command) *Credential {
	cred := &Credential{cmd: cmd, assumeFlagParsed: true}
	cmd.PersistentFlags().StringVarP(&cred.userFlag, "username", "u", "", "Username")
	cmd.PersistentFlags().StringVarP(&cred.passFlag, "password", "", "", "Password")
	return cred
}

func NewCredential(flagSet *flag.FlagSet) *Credential {
	cred := &Credential{}
	//	glog.Infof("XXX Adding username to flagset %P", flagSet)
	cred.flagSet = flagSet
	flagSet.StringVar(&cred.userFlag, "username", "", "Username")
	flagSet.StringVar(&cred.passFlag, "password", "", "Password")
	config.SetDefault(UsernameKey, "")
	config.SetDefault(PasswordKey, "")
	return cred
}

// GetPasswd gets password from stdin.
func GetPasswd() (string, error) {
	fmt.Print("Password: ")
	bytePassword, err := terminal.ReadPassword(syscall.Stdin)
	fmt.Println()
	if err != nil {
		return "", err
	}
	password := string(bytePassword)
	return password, nil
}

// Initialize constructs appropriate Credential structure based on
// provided data, which includes, in the following precedence (later
// superseding earlier):
// * In case of username/password auth:
//   1. As keys UsernameKey and PasswordKey in ~/.romana.yaml file
//   2. As environment variables whose names are UsernameKey and PasswordKey values
//   3. As --username and --password command-line flags.
//      If --username flag is specified but --password flag is omitted,
//      the user will be prompted for the password.
// Notes:
// 1. The first two precedence steps (~/.romana.yaml and environment variables)
//    are taken care by the config module (github.com/spf13/viper)
// 2. If flag.Parsed() is false at the time of this call, the command-line values are
//    ignored.
//
func (c *Credential) Initialize() error {
	username := config.GetString(UsernameKey)
	password := config.GetString(PasswordKey)
	if c.assumeFlagParsed || c.flagSet.Parsed() {
		if c.userFlag != "" {
			username = c.userFlag
			if c.passFlag == "" {
				// Ask for password
				var err error
				password, err = GetPasswd()
				if err != nil {
					return err
				}
			} else {
				password = c.passFlag
			}
		}
	}
	if username != "" {
		c.Username = username
		c.Password = password
		c.Type = CredentialUsernamePassword
	} else {
		// For now, credential is None if not specified
		c.Type = CredentialNone
	}
	return nil
}

// AuthZChecker takes a user and outputs whether the user is allowed to
// access a resource. If defined on a Route, it will be automatically invoked
// by wrapHandler(), which will provide RestContext.
type AuthZChecker func(ctx RestContext) bool

// AuthMiddleware wrapper for auth.
type AuthMiddleware struct {
	PublicKey   *rsa.PublicKey
	AllowedURLs []string
}

// NewAuthMiddleware creates new AuthMiddleware to use.
// Its behavior depends on whether it is for root (in which case
// the public key is gotten from the config file) or another
// service (in which case the public key is gotten from the root).
func NewAuthMiddleware(service Service) (AuthMiddleware, error) {
	authMiddleware := AuthMiddleware{}
	return authMiddleware, nil
	//	var err error
	//
	//	// If we are in the Root service...
	//	if service.Name() == ServiceNameRoot {
	//		// Really it would be most convenient to just use root.Root.publicKey but that
	//		// would create a circular import dependency.
	//		fullConfig := config.ServiceSpecific[FullConfigKey].(Config)
	//		rootConfig := fullConfig.Services[ServiceNameRoot].ServiceSpecific
	//		auth, err := ToBool(rootConfig["auth"])
	//		if err != nil {
	//			return authMiddleware, err
	//		}
	//		if auth {
	//			// If authentication is on, get the public key from local file
	//			// and parse and store it.
	//			publicKeyLocation := config.Common.Api.AuthPublic
	//			log.Debugf("Creating AuthMiddleware for Root: reading public key from %s", publicKeyLocation)
	//			data, err := ioutil.ReadFile(publicKeyLocation)
	//			if err != nil {
	//				return authMiddleware, err
	//			}
	//			key, err := jwt.ParseRSAPublicKeyFromPEM(data)
	//			if err != nil {
	//				log.Errorf("Error parsing RSA public key from %s: %T: %s", publicKeyLocation, err, err)
	//				return authMiddleware, err
	//			}
	//			authMiddleware.PublicKey = key
	//			// These URLs for Root are allowed to be accessed w/o authentication
	//			authMiddleware.AllowedURLs = []string{"/", "/auth", "/publicKey"}
	//		} else {
	//			// If the authentication is not turned on, just
	//			// set this to nil
	//			authMiddleware.PublicKey = nil
	//		}
	//		return authMiddleware, nil
	//	}
	//	// This is NOT root service - in this path
	//	// we are constructing AuthMiddleware for some other service.
	//	// So, first, get the public key to verify tokens with
	//	// from Root:
	//	authMiddleware.PublicKey, err = client.GetPublicKey()
	//	if err != nil {
	//		return authMiddleware, err
	//	}
	//	return authMiddleware, nil
}

// Keyfunc implements jwt.Keyfunc (https://godoc.org/github.com/dgrijalva/jwt-go#Keyfunc)
// by returning the public key
func (am AuthMiddleware) Keyfunc(*jwt.Token) (interface{}, error) {
	return am.PublicKey, nil
}

// ServeHTTP implements the middleware contract as follows:
//  1. If the path of request is one of the AllowedURLs, then this is a no-op.
//  2 Otherwise, checks token from request. If the token is not valid,
//  returns a 403 FORBIDDEN status.
func (am AuthMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request, next http.HandlerFunc) {
	for _, url := range am.AllowedURLs {
		if request.URL.Path == url {
			// If the requested path is one that the AuthMiddleware
			// knows should be allowed to access without authentication,
			// let everyone through -- which is to say, say that for this
			// request the user has Admin role.
			context.Set(request, ContextKeyUser, DefaultAdminUser)
			next(writer, request)
			return
		}
	}

	contentType := writer.Header().Get("Content-Type")
	marshaller := ContentTypeMarshallers[contentType]

	if am.PublicKey == nil {
		// If PublicKey is nil, it means auth is not on. So for simplicity,
		// say that any user is admin.
		context.Set(request, ContextKeyUser, DefaultAdminUser)
	} else {
		headerToken := request.Header.Get("Authorization")
		user := &User{}
		token, err := jwt.ParseWithClaims(headerToken, user, am.Keyfunc)
		log.Debugf("Token received from %s: %v", headerToken, token)

		if err != nil {
			writer.WriteHeader(http.StatusForbidden)
			httpErr := NewHttpError(http.StatusForbidden, fmt.Sprintf("Error accessing %s: %s", request.URL.Path, err.Error()))
			outData, _ := marshaller.Marshal(httpErr)
			writer.Write(outData)
			return
		}
		if !token.Valid {
			writer.WriteHeader(http.StatusForbidden)
			httpErr := NewHttpError(http.StatusForbidden, fmt.Sprintf("Invalid token in request to %s", request.URL.Path))
			outData, _ := marshaller.Marshal(httpErr)
			writer.Write(outData)
			return
		}
		log.Debugf("Token parsed: %+v vs %+v", token.Claims, user)
		context.Set(request, ContextKeyUser, *user)
	}
	next(writer, request)
}
