package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"syscall"
)

type Command struct {
	Short string
	Long string
	Run string
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

func execShell(shellCmd string) error {
	sh, err := exec.LookPath("sh")

	if err != nil {
		return err
	}

	args := []string{"sh", "-c", shellCmd}
	env := os.Environ()

	err = syscall.Exec(sh, args, env)

	if err != nil {
		return err
	}

	return nil
}

func buildCommands(parentCmd *cobra.Command, config *map[string]Command) {
	for use, command := range *config {
		parentCmd.AddCommand(&cobra.Command{
			Use:   use,
			Short: command.Short,
			Long:  command.Long,
			Run: func(cmd *cobra.Command, args []string) {
				if err := execShell(command.Run); err != nil {
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
