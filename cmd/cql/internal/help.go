/*
 * Copyright 2018-2019 The CovenantSQL Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package internal

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
)

const name = "cql"

var (
	// Version of command, set by main func of version
	Version = "unknown"
)

// CmdVersion is cql version command entity.
var CmdVersion = &Command{
	UsageLine: "cql version",
	Short:     "show build version information",
	Long: `
Use "cql help <command>" for more information about a command.
`,
	Flag:       flag.NewFlagSet("", flag.ExitOnError),
	CommonFlag: flag.NewFlagSet("", flag.ExitOnError),
	DebugFlag:  flag.NewFlagSet("", flag.ExitOnError),
}

// CmdHelp is cql help command entity.
var CmdHelp = &Command{
	UsageLine: "cql help [command]",
	Short:     "show help of sub commands",
	Long: `
Use "cql help <command>" for more information about a command.
`,
	Flag:       flag.NewFlagSet("", flag.ExitOnError),
	CommonFlag: flag.NewFlagSet("", flag.ExitOnError),
	DebugFlag:  flag.NewFlagSet("", flag.ExitOnError),
}

func init() {
	CmdVersion.Run = runVersion
	CmdHelp.Run = runHelp
}

// PrintVersion prints program git version.
func PrintVersion(printLog bool) string {
	version := fmt.Sprintf("%v %v %v %v %v\n",
		name, Version, runtime.GOOS, runtime.GOARCH, runtime.Version())

	if printLog {
		ConsoleLog.Debugf("cql build: %s\n", version)
	}

	return version
}

func runVersion(cmd *Command, args []string) {
	fmt.Print(PrintVersion(false))
}

func printParamHelp(flagSet *flag.FlagSet) {
	if flagSet.Name() != "" {
		_, _ = fmt.Fprintf(os.Stdout, "\n%s:\n", flagSet.Name())
	}
	flagSet.SetOutput(os.Stdout)
	flagSet.PrintDefaults()
}

func printCommandHelp(cmd *Command) {
	_, _ = fmt.Fprintf(os.Stdout, "usage: %s\n", cmd.UsageLine)
	_, _ = fmt.Fprintf(os.Stdout, cmd.Long)

	if cmd.Flag != nil {
		printParamHelp(cmd.Flag)
	}
	if cmd.CommonFlag != nil {
		printParamHelp(cmd.CommonFlag)
	}
	if cmd.DebugFlag != nil {
		printParamHelp(cmd.DebugFlag)
	}
}

func runHelp(cmd *Command, args []string) {
	if l := len(args); l != 1 {
		if l > 1 {
			// Don't support multiple commands
			SetExitStatus(2)
		}
		MainUsage()
	}

	cmdName := args[0]
	for _, command := range CqlCommands {
		if command.Name() != cmdName {
			continue
		}
		printCommandHelp(command)
		return
	}

	//Not support command
	SetExitStatus(2)
	MainUsage()
}

// MainUsage prints cql base help
func MainUsage() {
	helpHead := `cql is a tool for managing CQL database.

Usage:

    cql <command> [params] [arguments]

The commands are:

`
	helpCommon := `
The common params for commands (except help and version) are:

`
	helpDebug := `
The debug params for commands (except help and version) are:

`
	helpTail := `
Use "cql help <command>" for more information about a command.
`

	output := bytes.NewBuffer(nil)
	output.WriteString(helpHead)
	for _, cmd := range CqlCommands {
		if cmd.Name() == "help" {
			continue
		}
		fmt.Fprintf(output, "\t%-10s\t%s\n", cmd.Name(), cmd.Short)
	}

	addCommonFlags(CmdHelp)
	addConfigFlag(CmdHelp)
	fmt.Fprint(output, helpCommon)
	CmdHelp.CommonFlag.SetOutput(output)
	CmdHelp.CommonFlag.PrintDefaults()
	fmt.Fprint(output, helpDebug)
	CmdHelp.DebugFlag.SetOutput(output)
	CmdHelp.DebugFlag.PrintDefaults()

	fmt.Fprint(output, helpTail)
	fmt.Fprintf(os.Stdout, output.String())
	Exit()
}
