package main

import "net/url"

type uiapihandler struct{}

var uiapi uiapihandler

// GetDatabases returns the list of monitored databases
func (uiapi uiapihandler) GetDatabases() (any, error) {
	return GetMonitoredDatabasesFromConfigDB()
}

// DeleteDatabase removes the database from the list of monitored databases
func (uiapi uiapihandler) DeleteDatabase(database string) error {
	_, err := configDb.Exec("DELETE FROM pgwatch3.monitored_db WHERE md_unique_name = $1", database)
	return err
}

// AddDatabase adds the database to the list of monitored databases
func (uiapi uiapihandler) AddDatabase(params url.Values) error {
	sql := `INSERT TO pgwatch3.monitored_db(
md_unique_name, md_preset_config_name, md_config, md_hostname, md_port, md_dbname, md_user, md_password, md_is_superuser)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := configDb.Exec(sql, params.Get("md_unique_name"), params.Get("md_preset_config_name"), params.Get("md_config"),
		params.Get("md_hostname"), params.Get("md_port"), params.Get("md_dbname"), params.Get("md_user"),
		params.Get("md_password"), params.Get("md_is_superuser"))
	return err
}
