package cmdopts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

type SourceCommand struct {
	owner   *Options
	Ping    SourcePingCommand    `command:"ping" description:"Try to connect to configured sources, report errors if any and then exit"`
	Resolve SourceResolveCommand `command:"resolve" description:"Connect to the configured source(s) and return resolved connection strings for the monitoring targets discovered"`
	List    SourceListCommand    `command:"list" description:"Print a list of all sources configured in the config file"`
	// PrintSQL  SourcePrintCommand `command:"print" description:"Get and print SQL for a given Source"`
}

func NewSourceCommand(owner *Options) *SourceCommand {
	return &SourceCommand{
		owner:   owner,
		Ping:    SourcePingCommand{owner: owner},
		Resolve: SourceResolveCommand{owner: owner},
		List:    SourceListCommand{owner: owner},
	}
}

type SourcePingResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type SourcePingResults []SourcePingResult

func (r SourcePingResults) PrintText(w io.Writer) error {
	for _, s := range r {
		if s.Error != "" {
			fmt.Fprintf(w, "FAIL:\t%s (%s)\n", s.Name, s.Error)
		} else {
			fmt.Fprintf(w, "OK:\t%s\n", s.Name)
		}
	}
	return nil
}

type SourcePingCommand struct {
	owner *Options
}

func (cmd *SourcePingCommand) Execute(args []string) error {
	err := cmd.owner.InitSourceReader(context.Background())
	if err != nil {
		return err
	}
	srcs, err := cmd.owner.SourcesReaderWriter.GetSources()
	if err != nil {
		return err
	}
	var foundSources sources.Sources
	if len(args) == 0 {
		foundSources = srcs
	} else {
		for _, name := range args {
			for _, s := range srcs {
				if s.Name == name {
					foundSources = append(foundSources, s)
				}
			}
		}
	}
	var e error
	var results SourcePingResults
	for _, s := range foundSources {
		var status, errMsg string
		switch s.Kind {
		case sources.SourcePatroni:
			_, e = sources.ResolveDatabasesFromPatroni(s)
		case sources.SourcePostgresContinuous:
			_, e = sources.ResolveDatabasesFromPostgres(s)
		default:
			mdb := sources.NewSourceConn(s)
			e = mdb.Connect(context.Background(), cmd.owner.Sources)
		}
		if e != nil {
			status = "FAIL"
			errMsg = e.Error()
		} else {
			status = "OK"
		}
		results = append(results, SourcePingResult{Name: s.Name, Status: status, Error: errMsg})
		err = errors.Join(err, e)
	}
	cmd.owner.Print(results)
	// err here specifies execution error, not configuration error
	// so we indicate it with a special exit code
	// but we still return nil to indicate that the command was executed
	cmd.owner.CompleteCommand(map[bool]int32{true: ExitCodeCmdError, false: ExitCodeOK}[err != nil])
	return nil
}

type SourceResolveResult struct {
	Name    string `json:"name"`
	ConnStr string `json:"conn_str"`
}

type SourceResolveResults []SourceResolveResult

func (r SourceResolveResults) PrintText(w io.Writer) error {
	for _, s := range r {
		fmt.Fprintf(w, "%s=%s\n", s.Name, s.ConnStr)
	}
	return nil
}

type SourceResolveCommand struct {
	owner *Options
}

func (cmd *SourceResolveCommand) Execute(args []string) error {
	err := cmd.owner.InitSourceReader(context.Background())
	if err != nil {
		return err
	}
	srcs, err := cmd.owner.SourcesReaderWriter.GetSources()
	if err != nil {
		return err
	}
	var foundSources sources.Sources
	if len(args) == 0 {
		foundSources = srcs
	} else {
		for _, name := range args {
			for _, s := range srcs {
				if s.Name == name {
					foundSources = append(foundSources, s)
				}
			}
		}
	}
	conns, err := foundSources.ResolveDatabases()
	if err != nil {
		return err
	}
	var connstr url.URL
	connstr.Scheme = "postgresql"
	var results SourceResolveResults
	for _, s := range conns {
		var cs string
		if s.ConnStr > "" {
			cs = s.ConnStr
		} else {
			connstr.Host = fmt.Sprintf("%s:%d", s.ConnConfig.ConnConfig.Host, s.ConnConfig.ConnConfig.Port)
			connstr.User = url.UserPassword(s.ConnConfig.ConnConfig.User, s.ConnConfig.ConnConfig.Password)
			connstr.Path = s.ConnConfig.ConnConfig.Database
			cs = connstr.String()
		}
		results = append(results, SourceResolveResult{Name: s.Name, ConnStr: cs})
	}
	cmd.owner.Print(results)
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}

type SourceListResults []string

func (r SourceListResults) PrintText(w io.Writer) error {
	for _, s := range r {
		fmt.Fprintln(w, s)
	}
	return nil
}

type SourceListCommand struct {
	owner *Options
}

func (cmd *SourceListCommand) Execute(args []string) error {
	err := cmd.owner.InitSourceReader(context.Background())
	if err != nil {
		return err
	}
	srcs, err := cmd.owner.SourcesReaderWriter.GetSources()
	if err != nil {
		return err
	}
	var results SourceListResults
	for _, s := range srcs {
		results = append(results, s.Name)
	}
	cmd.owner.Print(results)
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}
