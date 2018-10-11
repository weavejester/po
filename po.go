package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
)

type Command struct {
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

func readCommandsFromYamlFile(path string, config *map[string]Command) error {
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

func buildCommands(parentCmd *cobra.Command, config *map[string]Command) {
	for use, command := range *config {
		parentCmd.AddCommand(&cobra.Command{
			Use: use,
			Run: func(cmd *cobra.Command, args []string) {
				shellCmd := exec.Command("sh", "-c", command.Run)
				shellCmd.Stdin = os.Stdin
				shellCmd.Stdout = os.Stdout
				shellCmd.Stderr = os.Stderr
				shellCmd.Run()
			},
		})
	}
}

func init() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	config := make(map[string]Command)

	if err := readCommandsFromYamlFile("po.yml", &config); err != nil {
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
