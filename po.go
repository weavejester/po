package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:     "po",
	Short:   "FIXME",
	Long:    "FIXME",
	Version: "0.0.1",
	Run:     func(c *cobra.Command, args []string) {},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
