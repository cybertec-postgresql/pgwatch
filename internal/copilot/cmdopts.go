package copilot

type CmdOpts struct {
	CopilotAddr string `long:"copilot-addr" description:"AI Copilot gRPC address" default:"localhost:50051"`
	Mode        string `long:"copilot-mode" description:"AI Copilot mode (normal/mock)" default:"normal"`
}
