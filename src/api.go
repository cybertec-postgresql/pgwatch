package main

import "net/url"

type uiapihandler struct{}

var uiapi uiapihandler

func get(v url.Values, key string) (res *string) {
	s := v.Get(key)
	if s == "" {
		return nil
	}
	return &s
}

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
func (uiapi uiapihandler) AddDatabase(params url.Values) error {
	sql := `INSERT INTO pgwatch3.monitored_db(
md_unique_name, md_preset_config_name, md_config, md_hostname, 
md_port, md_dbname, md_user, md_password, md_is_superuser, md_is_enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := configDb.Exec(sql, get(params, "md_unique_name"), get(params, "md_preset_config_name"),
		get(params, "md_config"), get(params, "md_hostname"), get(params, "md_port"),
		get(params, "md_dbname"), get(params, "md_user"), get(params, "md_password"),
		get(params, "md_is_superuser"), get(params, "md_is_enabled"))
	return err
}

// UpdateDatabase updates the monitored database information
func (uiapi uiapihandler) UpdateDatabase(database string, params url.Values) error {
	sql := `UPDATE pgwatch3.monitored_db SET
md_unique_name = $1, md_preset_config_name = $2, md_config = $3, md_hostname = $4, 
md_port = $5, md_dbname = $6, md_user = $7, md_password = $8, md_is_superuser = $9,
md_is_enabled = $10
WHERE md_unique_name = $11`
	_, err := configDb.Exec(sql,
		get(params, "md_unique_name"), get(params, "md_preset_config_name"), get(params, "md_config"),
		get(params, "md_hostname"), get(params, "md_port"), get(params, "md_dbname"), get(params, "md_user"),
		get(params, "md_password"), get(params, "md_is_superuser"), get(params, "md_is_enabled"), database)
	return err
}
