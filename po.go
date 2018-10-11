package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type Command struct {
	Short string
	Long  string
	Args  []string
	Run   string
}

var rootCmd = &cobra.Command{
	Use:     "po",
	Short:   "FIXME",
	Long:    "FIXME",
	Version: "0.0.1",
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

func argVarName(def string) string {
	return strings.TrimRight(def, "+*?")
}

func lastChar(s string) byte {
	return s[len(s)-1]
}

func envVarPair(def string, vals ...string) string {
	return fmt.Sprintf("%s=%s", argVarName(def), strings.Join(vals, " "))
}

func argEnvVars(defs []string, args []string) []string {
	env := make([]string, len(defs))

	for i := 0; i < len(args); i++ {
		def := defs[i]
		last := lastChar(def)

		if last == '*' || last == '+' {
			env[i] = envVarPair(def, args[i:]...)
			break
		} else {
			env[i] = envVarPair(def, args[i])
		}
	}

	return env
}

func minArgLength(defs []string) int {
	minLength := 0

	for _, def := range defs {
		last := lastChar(def)

		if last != '?' && last != '*' {
			minLength++
		}
	}

	return minLength
}

func hasArgMaxLength(defs []string) bool {
	for _, def := range defs {
		last := lastChar(def)

		if last == '*' || last == '+' {
			return false
		}
	}

	return true
}

func argsMatchDefs(defs []string) cobra.PositionalArgs {
	minLength := minArgLength(defs)
	maxLength := len(defs)
	hasMaxLength := hasArgMaxLength(defs)
	hasExactLength := hasMaxLength && minLength == maxLength

	return func(cmd *cobra.Command, args []string) error {
		if hasExactLength && len(args) != maxLength {
			return fmt.Errorf("requires exactly %d arguments", maxLength)
		} else if hasMaxLength && (len(args) < minLength || len(args) > maxLength) {
			return fmt.Errorf("requires between %d and %d arguments", minLength, maxLength)
		} else if len(args) < minLength {
			return fmt.Errorf("requires at least %d arguments", minLength)
		} else if hasMaxLength && len(args) > maxLength {
			return fmt.Errorf("requires at most %d arguments", maxLength)
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

func formatArgDef(def string) string {
	def = strings.ToUpper(def)

	switch lastChar(def) {
	case '?':
		return fmt.Sprintf("[%s]", def)
	case '+':
		return fmt.Sprintf("[%s...]", def)
	case '*':
		return fmt.Sprintf("%s...", def)
	default:
		return def
	}
}

func formatUsage(name string, command *Command) string {
	usageArgs := name

	for _, arg := range command.Args {
		usageArgs += " " + formatArgDef(arg)
	}

	return usageArgs
}

func buildCommands(parentCmd *cobra.Command, config *map[string]Command) {
	for name, command := range *config {
		parentCmd.AddCommand(&cobra.Command{
			Use:   formatUsage(name, &command),
			Short: command.Short,
			Long:  command.Long,
			Args:  argsMatchDefs(command.Args),
			Run: func(cmd *cobra.Command, args []string) {
				env := append(os.Environ(), argEnvVars(command.Args, args)...)

				if err := execShell(command.Run, env); err != nil {
					log.Fatalf("error: %v", err)
				}
			},
		})
	}
}

func init() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	config := make(map[string]Command)

	if err := readYamlFile("po.yml", &config); err != nil {
		log.Fatalf("error: %v", err)
	}

	buildCommands(rootCmd, &config)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
