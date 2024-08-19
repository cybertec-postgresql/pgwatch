package webserver

// WebUICmdOpts specifies the internal web UI server command-line options
type WebUICmdOpts struct {
	WebAddr     string `long:"web-addr" mapstructure:"web-addr" description:"TCP address in the form 'host:port' to listen on" default:":8080" env:"PW3_WEBADDR"`
	WebUser     string `long:"web-user" mapstructure:"web-user" description:"Admin login" env:"PW3_WEBUSER"`
	WebPassword string `long:"web-password" mapstructure:"web-password" description:"Admin password" env:"PW3_WEBPASSWORD"`
}
