package webserver

// CmdOpts specifies the internal web UI server command-line options
type CmdOpts struct {
	WebDisable  bool   `long:"web-disable" mapstructure:"web-disable" description:"Disable the web UI" env:"PW_WEBDISABLE"`
	WebAddr     string `long:"web-addr" mapstructure:"web-addr" description:"TCP address in the form 'host:port' to listen on" default:":8080" env:"PW_WEBADDR"`
	WebUser     string `long:"web-user" mapstructure:"web-user" description:"Admin login" env:"PW_WEBUSER"`
	WebPassword string `long:"web-password" mapstructure:"web-password" description:"Admin password" env:"PW_WEBPASSWORD"`
}
