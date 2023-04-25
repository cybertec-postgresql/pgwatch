package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type uiapihandler struct{}

var uiapi uiapihandler

// AddPreset adds the preset to the list of available presets
func (uiapi uiapihandler) AddPreset(params []byte) error {
	sql := `INSERT INTO pgwatch3.preset_config(pc_name, pc_description, pc_config) VALUES ($1, $2, $3)`
	var m map[string]any
	err := json.Unmarshal(params, &m)
	if err == nil {
		config, _ := json.Marshal(m["pc_config"])
		_, err = configDb.Exec(sql, m["pc_name"], m["pc_description"], config)
	}
	return err
}

// GetPresets returns the list of available presets
func (uiapi uiapihandler) GetPresets() (res string, err error) {
	sql := `select coalesce(jsonb_agg(to_jsonb(p)), '[]') from pgwatch3.preset_config p`
	err = configDb.Get(&res, sql)
	return
}

// DeletePreset removes the preset from the configuration
func (uiapi uiapihandler) DeletePreset(name string) error {
	_, err := configDb.Exec("DELETE FROM pgwatch3.preset_config WHERE pc_name = $1", name)
	return err
}

// GetMetrics returns the list of metrics
func (uiapi uiapihandler) GetMetrics() (res string, err error) {
	sql := `select coalesce(jsonb_agg(to_jsonb(m)), '[]') from metric m`
	err = configDb.Get(&res, sql)
	return
}

// DeleteMetric removes the metric from the configuration
func (uiapi uiapihandler) DeleteMetric(id int) error {
	_, err := configDb.Exec("DELETE FROM pgwatch3.metric WHERE m_id = $1", id)
	return err
}

// AddMetric adds the metric to the list of stored metrics
func (uiapi uiapihandler) AddMetric(params []byte) error {
	sql := `INSERT INTO pgwatch3.metric(
m_name, m_pg_version_from, m_sql, m_comment, m_is_active, m_is_helper, 
m_master_only, m_standby_only, m_column_attrs, m_sql_su)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	var m map[string]any
	err := json.Unmarshal(params, &m)
	if err == nil {
		_, err = configDb.Exec(sql, m["m_name"], m["m_pg_version_from"],
			m["m_sql"], m["m_comment"], m["m_is_active"],
			m["m_is_helper"], m["m_master_only"], m["m_standby_only"],
			m["m_column_attrs"], m["m_sql_su"])
	}
	return err
}

// GetDatabases returns the list of monitored databases
func (uiapi uiapihandler) GetDatabases() (res string, err error) {
	sql := `select coalesce(jsonb_agg(to_jsonb(db)), '[]') from monitored_db db`
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

// UpdateMetric updates the stored metric information
func (uiapi uiapihandler) UpdateMetric(id int, params []byte) error {
	fields, values, err := paramsToFieldValues(params)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf(`UPDATE pgwatch3.metric SET %s WHERE m_id = $1`, strings.Join(fields, ","))
	values = append([]any{id}, values...)
	_, err = configDb.Exec(sql, values...)
	return err
}

// UpdateDatabase updates the monitored database information
func (uiapi uiapihandler) UpdateDatabase(database string, params []byte) error {
	fields, values, err := paramsToFieldValues(params)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf(`UPDATE pgwatch3.monitored_db SET %s WHERE md_unique_name = $1`, strings.Join(fields, ","))
	values = append([]any{database}, values...)
	_, err = configDb.Exec(sql, values...)
	return err
}

// paramsToFieldValues tranforms JSON body into arrays to be used in UPDATE statement
func paramsToFieldValues(params []byte) (fields []string, values []any, err error) {
	var paramsMap map[string]any
	err = json.Unmarshal(params, &paramsMap)
	if err != nil {
		return
	}
	i := 2 // start with the second parameter number, first is reserved for WHERE key value
	for k, v := range paramsMap {
		fields = append(fields, fmt.Sprintf("%s = $%d", quoteIdent(k), i))
		values = append(values, v)
		i++
	}
	return
}

func quoteIdent(s string) string {
	return `"` + strings.Replace(s, `"`, `""`, -1) + `"`
}
