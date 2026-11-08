//go:build gosqlite

package db

/*
	KONJIYAMI Implant Framework
	Copyright (C) 2021  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"konjiyami/server/configs"
	"konjiyami/server/db/gosqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func sqliteClient(dbConfig *configs.DatabaseConfig) *gorm.DB {
	dsn, err := dbConfig.DSN()
	if err != nil {
		panic(err)
	}
	clientLog.Infof("sqlite -> %s", dsn)

	dbClient, err := gorm.Open(gosqlite.Open(dsn), &gorm.Config{
		Logger: getGormLogger(dbConfig),
	})
	if err != nil {
		panic(err)
	}

	sqlDB, err := dbClient.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.Exec("PRAGMA journal_mode=WAL")
		sqlDB.Exec("PRAGMA busy_timeout=5000")
	}

	return dbClient
}

