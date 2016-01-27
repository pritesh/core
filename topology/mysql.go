// Copyright (c) 2016 Pani Networks
// All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.
package topology

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"log"

	"errors"

	_ "github.com/go-sql-driver/mysql"

	"github.com/romana/core/common"

	"strconv"
)

type topoStore struct {
	common.Store
}

func (mysqlStore *mysqlStore) findHost(id uint64) (Host, error) {
	host := Host{}
	mysqlStore.db.Where("id = ?", id).First(&host)
	err := common.MakeMultiError(mysqlStore.db.GetErrors())
	if err != nil {
		return host, err
	}
	return host, nil
}

func (mysqlStore *mysqlStore) listHosts() ([]Host, error) {
	var hosts []Host
	log.Println("In listHosts()")
	mysqlStore.db.Find(&hosts)
	err := common.MakeMultiError(mysqlStore.db.GetErrors())
	if err != nil {
		return nil, err
	}
	log.Println("MySQL found hosts:", hosts)
	return hosts, nil
}

func (mysqlStore *mysqlStore) addHost(host *Host) (string, error) {
	mysqlStore.db.NewRecord(*host)
	mysqlStore.db.Create(host)
	err := common.MakeMultiError(mysqlStore.db.GetErrors())
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(host.Id, 10), nil
}


func (mysqlStore *mysqlStore) createSchema(force bool) error {
	log.Println("in createSchema(", force, ")")
	// Connect to mysql database
	schemaName := mysqlStore.info.Database
	mysqlStore.info.Database = "mysql"
	mysqlStore.setConnString()

	err := mysqlStore.connect()

	if err != nil {
		return err
	}
	var sql string

	if force {
		sql = fmt.Sprintf("DROP DATABASE IF EXISTS %s", schemaName)
		res, err := mysqlStore.db.DB().Exec(sql)
		if err != nil {
			return err
		}

		rows, _ := res.RowsAffected()
		log.Println(sql, ": ", rows)
	}

	sql = fmt.Sprintf("CREATE DATABASE %s", schemaName)
	res, err := mysqlStore.db.DB().Exec(sql)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	log.Println(sql, ": ", rows)
	mysqlStore.info.Database = schemaName
	mysqlStore.setConnString()
	err = mysqlStore.connect()
	if err != nil {
		return err
	}
	mysqlStore.db.CreateTable(&common.Datacenter{})
	mysqlStore.db.CreateTable(&Tor{})
	mysqlStore.db.CreateTable(&Host{})
	errs := mysqlStore.db.GetErrors()
	log.Println("Errors", errs)
	err2 := common.MakeMultiError(errs)

	if err2 != nil {
		return err2
	}
	return nil

}
