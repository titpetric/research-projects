# Commandtrx - Tracking commands as tool execution inside a system

For many of us developers, local testing is a practice to encourage and
maintain. That said, we have a tendency to leave the heavy lifting to
CI/CD systems, without really having insight into what tests and
commands have ran in a developmer system.

The popularization of agentic code assistants leaves an observability
gap. The agent invokes tools like `go build`, `go test`, `docker build`,
`docker compose up` and many others. Even without agents, there are a
set of other tools that are part of the development lifecycle, like
`make`, `task`, `atkins`, `goimports`, `gopls` and others.

I want a generic go program (main.go + a few files grouping definitions by name).
I want the program to use ebpf to trace execution logs of the running
system.
I want to install the tool on my system via docker (--privileged)

I am open to considering other options.

The intent of the tool is to:

- catch an event for every command executed in the system
- summarize the command without arguments (up until the first `-` prefixed argument flag)
- filter for known commands: `go`, `git`, `docker`, ...

Arguments:

- `--filter go,git,docker` (csv argument), if not provided - all commands are caught,

The tool should emit which command was run into stdout.
