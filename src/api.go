package main

import (
	"encoding/json"
)

type uiapihandler struct{}

var uiapi uiapihandler

// GetDatabases returns the list of monitored databases
func (uiapi uiapihandler) GetDatabases() (res string, err error) {
	sql := `select jsonb_agg(to_jsonb(db)) from monitored_db db`
	err = configDb.Get(&res, sql)
	return
}

// DeleteDatabase removes the database from the list of monitored databases
func (uiapi uiapihandler) DeleteDatabase(database string) error {
	_, err := configDb.Exec("DELETE FROM pgwatch3.monitored_db WHERE md_unique_name = $1", database)
	return err
}

// AddDatabase adds the database to the list of monitored databases
func (uiapi uiapihandler) AddDatabase(params []byte) error {
	sql := `INSERT INTO pgwatch3.monitored_db(
md_unique_name, md_preset_config_name, md_config, md_hostname, 
md_port, md_dbname, md_user, md_password, md_is_superuser, md_is_enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	var m map[string]any
	err := json.Unmarshal(params, &m)
	if err == nil {
		_, err = configDb.Exec(sql, m["md_unique_name"], m["md_preset_config_name"],
			m["md_config"], m["md_hostname"], m["md_port"],
			m["md_dbname"], m["md_user"], m["md_password"],
			m["md_is_superuser"], m["md_is_enabled"])
	}
	return err
}

// UpdateDatabase updates the monitored database information
func (uiapi uiapihandler) UpdateDatabase(database string, params []byte) error {
	sql := `UPDATE pgwatch3.monitored_db SET
md_unique_name = $1, md_preset_config_name = $2, md_config = $3, md_hostname = $4, 
md_port = $5, md_dbname = $6, md_user = $7, md_password = $8, md_is_superuser = $9,
md_is_enabled = $10
WHERE md_unique_name = $11`
	var m map[string]any
	err := json.Unmarshal(params, &m)
	if err == nil {
		_, err = configDb.Exec(sql,
			m["md_unique_name"], m["md_preset_config_name"], m["md_config"],
			m["md_hostname"], m["md_port"], m["md_dbname"], m["md_user"],
			m["md_password"], m["md_is_superuser"], m["md_is_enabled"], database)
	}
	return err
}
