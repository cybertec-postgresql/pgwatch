package cmdopts

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

type SourceCommand struct {
	owner   *Options
	Ping    SourcePingCommand    `command:"ping" description:"Try to connect to configured sources, report errors if any and then exit"`
	Resolve SourceResolveCommand `command:"resolve" description:"Connect to the configured source(s) and return resolved connection strings for the monitoring targets discovered"`
	// PrintSQL  SourcePrintCommand `command:"print" description:"Get and print SQL for a given Source"`
}

func NewSourceCommand(owner *Options) *SourceCommand {
	return &SourceCommand{
		owner:   owner,
		Ping:    SourcePingCommand{owner: owner},
		Resolve: SourceResolveCommand{owner: owner},
	}
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
	for _, s := range foundSources {
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
			fmt.Printf("FAIL:\t%s (%s)\n", s.Name, e)
		} else {
			fmt.Printf("OK:\t%s\n", s.Name)
		}
		err = errors.Join(err, e)
	}
	// err here specifies execution error, not configuration error
	// so we indicate it with a special exit code
	// but we still return nil to indicate that the command was executed
	cmd.owner.CompleteCommand(map[bool]int32{true: ExitCodeCmdError, false: ExitCodeOK}[err != nil])
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
	for _, s := range conns {
		if s.ConnStr > "" {
			fmt.Printf("%s=%s\n", s.Name, s.ConnStr)
		} else {
			connstr.Host = fmt.Sprintf("%s:%d", s.ConnConfig.ConnConfig.Host, s.ConnConfig.ConnConfig.Port)
			connstr.User = url.UserPassword(s.ConnConfig.ConnConfig.User, s.ConnConfig.ConnConfig.Password)
			connstr.Path = s.ConnConfig.ConnConfig.Database
			fmt.Printf("%s=%s\n", s.Name, connstr.String())
		}
	}
	cmd.owner.CompleteCommand(ExitCodeOK)
	return nil
}
