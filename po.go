package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
)

var rootCmd = &cobra.Command{
	Use:     "po",
	Short:   "FIXME",
	Long:    "FIXME",
	Version: "0.0.1",
	Run:     func(c *cobra.Command, args []string) {
		dat, err := ioutil.ReadFile("po.yml")

		if err != nil {
			log.Fatal(err)
		}

		m := make(map[interface{}]interface{})

		err = yaml.Unmarshal(dat, &m)

		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(m)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
