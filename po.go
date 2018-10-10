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

var rootCmd = &cobra.Command{
	Use:     "po",
	Short:   "FIXME",
	Long:    "FIXME",
	Version: "0.0.1",
	Run: func(c *cobra.Command, args []string) {
	},
}

func readCommandsFromYamlFile(path string, m *map[string]string) error {
	dat, err := ioutil.ReadFile(path)

	if err != nil {
		return err
	}

	err = yaml.Unmarshal(dat, &m)

	if err != nil {
		return err
	}

	return nil
}

func buildCommands(parentCmd *cobra.Command, m *map[string]string) {
	for k, v := range *m {
		cmd := &cobra.Command{
			Use: k,
			Run: func(c *cobra.Command, args []string) {
				shellCmd := exec.Command("sh", "-c", v)
				shellCmd.Stdin = os.Stdin
				shellCmd.Stdout = os.Stdout
				shellCmd.Stderr = os.Stderr
				shellCmd.Run()
			},
		}

		parentCmd.AddCommand(cmd)
	}
}

func init() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	m := make(map[string]string)

	if err := readCommandsFromYamlFile("po.yml", &m); err != nil {
		log.Fatalf("error: %v", err)
	}

	buildCommands(rootCmd, &m)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
