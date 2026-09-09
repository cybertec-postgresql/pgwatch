package webserver

const (
	WebDisableAll string = "all"
	WebDisableUI  string = "ui"
)

// DefaultCORSOrigin is the origin allowed by the CORS middleware unless
// --web-cors-origin or webserver.WithCORSOrigin says otherwise.
const DefaultCORSOrigin = "http://localhost:4000" // check internal/webui/.env

// CmdOpts specifies the internal web UI server command-line options
type CmdOpts struct {
	WebDisable    string `long:"web-disable" mapstructure:"web-disable" description:"Disable REST API and/or web UI" env:"PW_WEBDISABLE" optional:"true" optional-value:"all" choice:"all" choice:"ui"`
	WebAddr       string `long:"web-addr" mapstructure:"web-addr" description:"TCP address in the form 'host:port' to listen on" default:":8080" env:"PW_WEBADDR"`
	WebBasePath   string `long:"web-base-path" mapstructure:"web-base-path" description:"Base path for web UI and API endpoints (e.g., 'pgwatch' for reverse proxy setups)" env:"PW_WEBBASEPATH"`
	WebUser       string `long:"web-user" mapstructure:"web-user" description:"Admin username" env:"PW_WEBUSER"`
	WebPassword   string `long:"web-password" mapstructure:"web-password" description:"Admin password" env:"PW_WEBPASSWORD"`
	WebCORSOrigin string `hidden:"true" long:"web-cors-origin" mapstructure:"web-cors-origin" description:"Origin allowed by the CORS middleware" default:"http://localhost:4000" env:"PW_WEBCORSORIGIN"`
}
