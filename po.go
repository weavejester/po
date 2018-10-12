package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type Argument struct {
	Var      string
	Short    string
	AtLeastP *int `yaml:"at_least"`
	AtMostP  *int `yaml:"at_most"`
}

func (arg *Argument) AtLeast() int {
	if arg.AtLeastP == nil {
		return 1
	} else {
		return *arg.AtLeastP
	}
}

func (arg *Argument) AtMost() int {
	if arg.AtMostP == nil {
		return arg.AtLeast()
	} else {
		return *arg.AtMostP
	}
}

type Flag struct {
	Desc    string
	Short   string
	Type    string
	Default string
}

type Command struct {
	Short string
	Long  string
	Args  []Argument
	Flags map[string]Flag
	Run   string
}

func (cmd *Command) MaxArgLength() int {
	length := 0
	for _, arg := range cmd.Args {
		l := len(arg.Var)
		if length < l {
			length = l
		}
	}
	return length
}

const minArgPadding = 8

func (cmd *Command) ArgPadding() int {
	padding := cmd.MaxArgLength()

	if padding < minArgPadding {
		return minArgPadding
	}
	return padding
}

var rootCmd = &cobra.Command{
	Use:           "po",
	Short:         "FIXME",
	Long:          "FIXME",
	Version:       "0.0.1",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
		os.Exit(0)
	},
}

func readYamlFile(path string, config *map[string]Command) error {
	dat, err := ioutil.ReadFile(path)

	if err != nil {
		return err
	}

	err = yaml.Unmarshal(dat, &config)

	if err != nil {
		return err
	}

	return nil
}

func minArgLength(defs []Argument) int {
	minLength := 0

	for _, def := range defs {
		minLength += def.AtLeast()
	}

	return minLength
}

func maxArgLength(defs []Argument) int {
	maxLength := 0

	for _, def := range defs {
		maxLength += def.AtMost()
	}

	return maxLength
}

func envVarPair(name string, vals []string) string {
	return fmt.Sprintf("%s=%s", name, strings.Join(vals, " "))
}

func argEnvVars(defs []Argument, args []string) []string {
	env := make([]string, len(defs))
	required := minArgLength(defs)
	a := 0

	for i, def := range defs {
		required -= def.AtLeast()
		aNext := a + def.AtMost()

		if aNext > len(args)-required {
			aNext = len(args)
		}

		env[i] = envVarPair(def.Var, args[a:aNext])
		a = aNext
	}

	return env
}

func visitFlagsWithValues(flags *pflag.FlagSet, fn func(*pflag.Flag)) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Changed || flag.DefValue != "" {
			fn(flag)
		}
	})
}

func flagValueOrDefault(flag *pflag.Flag) string {
	if flag.Changed {
		return flag.Value.String()
	}
	return flag.DefValue
}

func countFlagsWithValues(flags *pflag.FlagSet) int {
	count := 0
	visitFlagsWithValues(flags, func(f *pflag.Flag) { count++ })
	return count
}

func flagEnvVars(flags *pflag.FlagSet) []string {
	env := make([]string, countFlagsWithValues(flags))
	i := 0

	visitFlagsWithValues(flags, func(f *pflag.Flag) {
		env[i] = fmt.Sprintf("%s=%s", f.Name, flagValueOrDefault(f))
	})

	return env
}

func argsMatchDefs(defs []Argument) cobra.PositionalArgs {
	minLength := minArgLength(defs)
	maxLength := maxArgLength(defs)

	return func(cmd *cobra.Command, args []string) error {
		switch {
		case minLength == 0 && maxLength == 0 && len(args) > 0:
			return fmt.Errorf("should have no arguments")
		case minLength == maxLength && len(args) != maxLength:
			return fmt.Errorf("requires exactly %d arguments", maxLength)
		case maxLength > 0 && minLength > 0 && (len(args) < minLength || len(args) > maxLength):
			return fmt.Errorf("requires between %d and %d arguments", minLength, maxLength)
		case maxLength > 0 && len(args) > maxLength:
			return fmt.Errorf("requires at most %d arguments", maxLength)
		case len(args) < minLength:
			return fmt.Errorf("requires at least %d arguments", minLength)
		}

		return nil
	}
}

func execShell(shellCmd string, env []string) error {
	sh, err := exec.LookPath("sh")

	if err != nil {
		return err
	}

	args := []string{"sh", "-c", shellCmd}

	err = syscall.Exec(sh, args, env)

	if err != nil {
		return err
	}

	return nil
}

func formatArgDef(def Argument) string {
	arg := strings.ToUpper(def.Var)

	if def.AtLeast() > 1 || def.AtMost() > 1 {
		arg = fmt.Sprintf("%s...", arg)
	}

	if def.AtLeast() < 1 {
		arg = fmt.Sprintf("[%s]", arg)
	}

	return arg
}

func formatUsage(name string, command *Command) string {
	usageArgs := name

	for _, arg := range command.Args {
		usageArgs += " " + formatArgDef(arg)
	}

	return usageArgs
}

func rightPad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds", padding)
	return fmt.Sprintf(template, s)
}

func argUsages(command *Command) string {
	usage := ""
	padding := command.ArgPadding()

	for _, arg := range command.Args {
		argvar := strings.ToUpper(arg.Var)
		usage += fmt.Sprintf("  %s %s\n", rightPad(argvar, padding), arg.Short)
	}

	return usage
}

func commandUsages(command *cobra.Command) string {
	usage := ""
	padding := command.NamePadding()

	for _, cmd := range command.Commands() {
		usage += fmt.Sprintf("  %s %s\n", rightPad(cmd.Name(), padding), cmd.Short)
	}

	return usage
}

func rootUsageFunc(rootCmd *cobra.Command) error {
	bold := color.New(color.Bold)
	out := rootCmd.OutOrStderr()

	bold.Fprintf(out, "USAGE\n")
	fmt.Fprintf(out, "  %s [COMMAND] [FLAGS]\n", rootCmd.CommandPath())

	if rootCmd.HasAvailableSubCommands() {
		bold.Fprintf(out, "\nCOMMANDS\n")
		fmt.Fprintf(out, commandUsages(rootCmd))
	}

	if rootCmd.HasAvailableLocalFlags() {
		bold.Fprintf(out, "\nFLAGS\n")
		fmt.Fprintf(out, rootCmd.LocalFlags().FlagUsages())
	}
	return nil
}

func makeUsageFunc(command *Command) func(*cobra.Command) error {
	bold := color.New(color.Bold)
	args := command.Args
	argUsageText := argUsages(command)

	return func(cobra *cobra.Command) error {
		out := cobra.OutOrStderr()

		bold.Fprintf(out, "USAGE\n")
		fmt.Fprintf(out, "  %s [FLAGS]\n", cobra.UseLine())

		if len(args) > 0 {
			bold.Fprintf(out, "\nARGUMENTS\n")
			fmt.Fprintf(out, argUsageText)
		}

		if cobra.HasAvailableLocalFlags() {
			bold.Fprintf(out, "\nFLAGS\n")
			fmt.Fprintf(out, cobra.LocalFlags().FlagUsages())
		}
		return nil
	}
}

func buildFlags(cmd *cobra.Command, flags map[string]Flag) error {
	for name, flag := range flags {
		switch flag.Type {
		case "string":
			cmd.Flags().StringP(name, flag.Short, flag.Default, flag.Desc)
		default:
			return fmt.Errorf("no such type: %v", flag.Type)
		}
	}
	return nil
}

func buildCommands(parentCmd *cobra.Command, config *map[string]Command) error {
	for name, command := range *config {
		argDefs := command.Args
		script := command.Run
		cmd := cobra.Command{
			Use:                   formatUsage(name, &command),
			Short:                 command.Short,
			Long:                  command.Long,
			Args:                  argsMatchDefs(argDefs),
			DisableFlagsInUseLine: true,
			Run: func(cmd *cobra.Command, args []string) {
				env := os.Environ()
				env = append(env, argEnvVars(argDefs, args)...)
				env = append(env, flagEnvVars(cmd.Flags())...)

				if err := execShell(script, env); err != nil {
					log.Fatalf("error: %v", err)
				}
			},
		}
		cmd.SetUsageFunc(makeUsageFunc(&command))

		if err := buildFlags(&cmd, command.Flags); err != nil {
			return err
		}
		parentCmd.AddCommand(&cmd)
	}
	return nil
}

func init() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	rootCmd.SetUsageFunc(rootUsageFunc)

	config := make(map[string]Command)

	if err := readYamlFile("po.yml", &config); err != nil {
		log.Fatalf("error: %v", err)
	}

	if err := buildCommands(rootCmd, &config); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func main() {
	if cmd, err := rootCmd.ExecuteC(); err != nil {
		out := cmd.OutOrStderr()
		boldRed := color.New(color.Bold, color.FgRed)
		boldRed.Fprintf(out, "ERROR")
		fmt.Fprintf(out, ": '%s' command %s\n", cmd.Name(), err)
		fmt.Fprintf(out, "Run '%v --help' for usage.\n", cmd.CommandPath())
		os.Exit(1)
	}
}
