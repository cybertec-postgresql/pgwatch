package main

type uiapihandler struct{}

var uiapi uiapihandler

func (uiapi uiapihandler) GetDatabases() (any, error) {
	return GetMonitoredDatabasesFromConfigDB()
}

func (uiapi uiapihandler) DeleteDatabase(database string) error {
	_, err := configDb.Exec("DELETE FROM pgwatch3.monitored_db WHERE md_unique_name = $1", database)
	return err
}
