package main

import (
	"crypto/sha1"
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
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

func (a *Argument) Merge(b *Argument) {
	if b.Var != "" {
		a.Var = b.Var
	}
	if b.Short != "" {
		a.Short = b.Short
	}
	if b.AtLeastP != nil {
		a.AtLeastP = b.AtLeastP
	}
	if b.AtMostP != nil {
		a.AtMostP = b.AtMostP
	}
}

type Flag struct {
	Desc    string
	Short   string
	Type    string
	Default string
}

func (a *Flag) Merge(b *Flag) {
	if b.Desc != "" {
		a.Desc = b.Desc
	}
	if b.Short != "" {
		a.Short = b.Short
	}
	if b.Type != "" {
		a.Type = b.Type
	}
	if b.Default != "" {
		a.Default = b.Default
	}
}

type Command struct {
	Short    string
	Long     string
	Args     []Argument
	Flags    map[string]Flag
	Run      string
	Commands map[string]Command
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

func mergeFlags(a map[string]Flag, b map[string]Flag) {
	for k, vb := range b {
		if va, ok := a[k]; ok {
			va.Merge(&vb)
		} else {
			a[k] = vb
		}
	}
}

func mergeCommands(a map[string]Command, b map[string]Command) {
	for k, vb := range b {
		if va, ok := a[k]; ok {
			va.Merge(&vb)
		} else {
			a[k] = vb
		}
	}
}

func (a *Command) Merge(b *Command) {
	if b.Short != "" {
		a.Short = b.Short
	}
	if b.Long != "" {
		a.Long = b.Long
	}
	if b.Run != "" {
		a.Run = b.Run
	}

	if len(b.Args) > 0 {
		a.Args = b.Args
	}

	mergeFlags(a.Flags, b.Flags)
	mergeCommands(a.Commands, b.Commands)
}

type Import struct {
	File string
	Url  string
}

func mergeVars(a map[string]string, b map[string]string) {
	for k, vb := range b {
		a[k] = vb
	}
}

type Config struct {
	Imports  []Import
	Vars     map[string]string
	Commands map[string]Command
}

func (a *Config) Merge(b *Config) {
	if a.Commands == nil {
		a.Commands = b.Commands
	} else if b.Commands != nil {
		mergeCommands(a.Commands, b.Commands)
	}

	if a.Vars == nil {
		a.Vars = b.Vars
	} else if b.Vars != nil {
		mergeVars(a.Vars, b.Vars)
	}
}

func parseConfig(dat []byte) (*Config, error) {
	var config Config

	if err := yaml.Unmarshal(dat, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func readConfig(reader io.Reader) (*Config, error) {
	dat, err := ioutil.ReadAll(reader)

	if err != nil {
		return nil, err
	}

	return parseConfig(dat)
}

func readConfigFile(path string) (*Config, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	return readConfig(file)
}

func readConfigFileIfExists(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	return readConfigFile(path)
}

func urlCacheFilename(url string) string {
	h := sha1.New()
	h.Write([]byte(url))

	return fmt.Sprintf("%x", h.Sum(nil))
}

func readUrlCache(url string) ([]byte, error) {
	userCacheDir, err := os.UserCacheDir()

	if err != nil {
		return nil, err
	}

	cachePath := filepath.Join(userCacheDir, "po", urlCacheFilename(url))

	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil, nil
	}

	return ioutil.ReadFile(cachePath)
}

func writeUrlCache(url string, dat []byte) error {
	userCacheDir, err := os.UserCacheDir()

	if err != nil {
		return err
	}

	cacheDir := filepath.Join(userCacheDir, "po")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(cacheDir, urlCacheFilename(url))

	return ioutil.WriteFile(path, dat, 0644)
}

func readConfigUrl(url string) (*Config, error) {
	dat, err := readUrlCache(url)

	if err != nil {
		return nil, err
	}

	if dat != nil {
		return parseConfig(dat)
	}

	resp, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	dat, err = ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	if err := writeUrlCache(url, dat); err != nil {
		return nil, err
	}

	return parseConfig(dat)
}

func userConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	} else {
		return filepath.Join(os.Getenv("HOME"), ".config")
	}
}

const configFileName = "po.yml"

func userConfigPath() string {
	return filepath.Join(userConfigDir(), "po", configFileName)
}

func isRootPath(path string) bool {
	return path == filepath.Join(path, "..")
}

func findProjectConfig() (string, error) {
	cwd, err := filepath.Abs(".")

	if err != nil {
		return "", err
	}

	for path := cwd; !isRootPath(path); path = filepath.Join(path, "..") {
		configPath := filepath.Join(path, configFileName)

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			return configPath, nil
		}
	}

	return "", nil
}

func findImportPath(importPath string, parents []Import) string {
	lastParent := parents[len(parents)-1]

	if lastParent.File == "" || path.IsAbs(importPath) {
		return importPath
	} else {
		return filepath.Join(filepath.Dir(lastParent.File), importPath)
	}
}

func readImport(imp Import, parents []Import) (*Config, error) {
	if imp.File != "" {
		return readConfigFile(findImportPath(imp.File, parents))
	} else {
		return readConfigUrl(imp.Url)
	}
}

func hasImport(haystack []Import, needle Import) bool {
	for _, imp := range haystack {
		if imp == needle {
			return true
		}
	}
	return false
}

func loadImports(config *Config, parents []Import) error {
	lastParent := parents[len(parents)-1]

	for _, imp := range config.Imports {
		if imp.File != "" && imp.Url != "" {
			return fmt.Errorf("cannot have an import with a file and a URL set")
		}

		if hasImport(parents, imp) {
			return fmt.Errorf("cyclic dependency in imports")
		}

		if imp.File != "" && lastParent.Url != "" {
			return fmt.Errorf("cannot load a file import referenced from a URL")
		}

		importedCfg, err := readImport(imp, parents)

		if err != nil {
			return err
		}

		parents = append(parents, imp)

		if err := loadImports(importedCfg, parents); err != nil {
			return err
		}

		parents = parents[:len(parents)-1]

		config.Merge(importedCfg)
	}

	return nil
}

func loadRootImports(config *Config, path string) error {
	return loadImports(config, []Import{Import{File: path}})
}

const projectRootEnvVar = "PO_PROJECT_ROOT"

func loadAllConfigs() (*Config, error) {
	userCfgPath := userConfigPath()
	userCfg, err := readConfigFile(userCfgPath)

	if err != nil {
		return nil, err
	}

	if userCfg != nil {
		if err := loadRootImports(userCfg, userCfgPath); err != nil {
			return nil, err
		}
	}

	projectCfgPath, err := findProjectConfig()

	if err != nil {
		return nil, err
	}

	os.Setenv(projectRootEnvVar, filepath.Dir(projectCfgPath))

	var projectCfg *Config

	if projectCfgPath != "" {
		projectCfg, err = readConfigFileIfExists(projectCfgPath)

		if err != nil {
			return nil, err
		}
	}

	if projectCfg != nil {
		if err := loadRootImports(projectCfg, projectCfgPath); err != nil {
			return nil, err
		}
	}

	switch {
	case userCfg == nil && projectCfg == nil:
		return nil, nil
	case userCfg == nil:
		return projectCfg, nil
	case projectCfg == nil:
		return userCfg, nil
	default:
		userCfg.Merge(projectCfg)
		return userCfg, nil
	}
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

func isFalseBoolFlag(f *pflag.Flag) bool {
	return f.Value.Type() == "bool" && f.Value.String() == "false"
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
		if isFalseBoolFlag(f) {
			return
		}
		env[i] = fmt.Sprintf("%s=%s", f.Name, flagValueOrDefault(f))
		i++
	})

	return env[:i]
}

func configEnvVars(config *Config) []string {
	if config.Vars == nil {
		return []string{}
	}

	env := make([]string, len(config.Vars))
	i := 0

	for k, v := range config.Vars {
		env[i] = fmt.Sprintf("%s=%s", k, v)
		i++
	}

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

func isRootCommand(cmd *cobra.Command) bool {
	return !strings.Contains(cmd.Name(), ":")
}

func rootCommandUsages(command *cobra.Command, prefix string) string {
	usage := ""
	padding := command.NamePadding()

	for _, cmd := range command.Commands() {
		if isRootCommand(cmd) {
			usage += fmt.Sprintf("%s%s %s\n", prefix, rightPad(cmd.Name(), padding), cmd.Short)
		}
	}

	return usage
}

func isSubCommand(parentCmd *cobra.Command, cmd *cobra.Command) bool {
	return strings.HasPrefix(cmd.Name(), parentCmd.Name()+":")
}

func hasSubCommands(parentCmd *cobra.Command, cmd *cobra.Command) bool {
	for _, subCmd := range parentCmd.Commands() {
		if isSubCommand(cmd, subCmd) {
			return true
		}
	}
	return false
}

func subCommandUsages(parentCmd *cobra.Command, cmd *cobra.Command) string {
	usage := ""
	padding := parentCmd.NamePadding()

	for _, subCmd := range parentCmd.Commands() {
		if isSubCommand(cmd, subCmd) {
			usage += fmt.Sprintf("  %s %s\n", rightPad(subCmd.Name(), padding), subCmd.Short)
		}
	}

	return usage
}

func makeUsageFunc(parentCmd *cobra.Command, command *Command) func(*cobra.Command) error {
	bold := color.New(color.Bold)
	args := command.Args
	run := command.Run
	argUsageText := argUsages(command)

	return func(cobra *cobra.Command) error {
		out := cobra.OutOrStderr()

		if run != "" {
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
			fmt.Println()
		}

		if hasSubCommands(rootCmd, cobra) {
			bold.Fprintf(out, "COMMANDS\n")
			fmt.Fprintf(out, subCommandUsages(parentCmd, cobra))
		}

		return nil
	}
}

func helpFunc(cmd *cobra.Command, args []string) {
	out := cmd.OutOrStderr()

	if cmd.Long != "" {
		fmt.Fprintf(out, "%s\n\n", strings.Trim(cmd.Long, "\n"))
	} else {
		fmt.Fprintf(out, "%s\n\n", strings.Trim(cmd.Short, "\n"))
	}

	cmd.Usage()
}

func parseInt(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	} else {
		return 0
	}
}

func parseBool(s string) bool {
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	} else {
		return false
	}
}

func buildFlags(cmd *cobra.Command, flags map[string]Flag) error {
	for name, flag := range flags {
		switch flag.Type {
		case "string":
			cmd.Flags().StringP(name, flag.Short, flag.Default, flag.Desc)
		case "int":
			cmd.Flags().IntP(name, flag.Short, parseInt(flag.Default), flag.Desc)
		case "bool":
			cmd.Flags().BoolP(name, flag.Short, parseBool(flag.Default), flag.Desc)
		default:
			return fmt.Errorf("no such type: %v", flag.Type)
		}
	}
	return nil
}

func makeRunFunc(config *Config, command *Command) func(*cobra.Command, []string) {
	if command.Run == "" {
		return nil
	}

	configEnv := configEnvVars(config)

	commandArgs := command.Args
	commandRun := command.Run

	return func(cmd *cobra.Command, args []string) {
		env := os.Environ()
		env = append(env, configEnv...)
		env = append(env, argEnvVars(commandArgs, args)...)
		env = append(env, flagEnvVars(cmd.Flags())...)

		if err := execShell(commandRun, env); err != nil {
			log.Fatalf("error: %v", err)
		}
	}
}

func buildCommand(config *Config, parentCmd *cobra.Command, name string, command *Command) (*cobra.Command, error) {
	cmd := cobra.Command{
		Use:                   formatUsage(name, command),
		Short:                 command.Short,
		Long:                  command.Long,
		Args:                  argsMatchDefs(command.Args),
		DisableFlagsInUseLine: true,
		Run:                   makeRunFunc(config, command),
	}
	cmd.SetUsageFunc(makeUsageFunc(parentCmd, command))
	cmd.SetHelpFunc(helpFunc)

	if err := buildFlags(&cmd, command.Flags); err != nil {
		return &cmd, err
	}

	for subname, subcommand := range command.Commands {
		_, err := buildCommand(config, parentCmd, name+":"+subname, &subcommand)

		if err != nil {
			return &cmd, err
		}
	}

	parentCmd.AddCommand(&cmd)
	return &cmd, nil
}

func buildCommandsFromConfig(config *Config, parentCmd *cobra.Command) error {
	for name, command := range config.Commands {
		_, err := buildCommand(config, parentCmd, name, &command)

		if err != nil {
			return err
		}
	}
	return nil
}

func deleteFilesInDir(dir string) error {
	files, err := ioutil.ReadDir(dir)

	if err != nil {
		return err
	}

	for _, file := range files {
		os.Remove(filepath.Join(dir, file.Name()))
	}

	return nil
}

func deleteCacheFiles() error {
	userCacheDir, err := os.UserCacheDir()

	if err != nil {
		return err
	}

	cacheDir := filepath.Join(userCacheDir, "po")

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return nil
	}

	return deleteFilesInDir(cacheDir)
}

func printError(cmd *cobra.Command, err error) {
	boldRed := color.New(color.Bold, color.FgRed)
	boldRed.Fprintf(os.Stderr, "ERROR")
	fmt.Fprintf(os.Stderr, " [%s]: %s\n", cmd.CommandPath(), err)
	fmt.Fprintf(os.Stderr, "Run '%v --help' for usage.\n", cmd.CommandPath())
}

func getRootBoolFlag(cmd *cobra.Command, name string) bool {
	value, err := cmd.Flags().GetBool(name)

	if err != nil {
		printError(cmd, err)
		os.Exit(1)
	}

	return value
}

var rootCmd = &cobra.Command{
	Use:           "po",
	Short:         "CLI for managing project-specific scripts",
	Version:       "0.0.1",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		refresh := getRootBoolFlag(cmd, "refresh")
		commands := getRootBoolFlag(cmd, "commands")

		switch {
		case refresh:
			if err := deleteCacheFiles(); err != nil {
				printError(cmd, err)
				os.Exit(1)
			}
		case commands:
			cmd.Printf(rootCommandUsages(cmd, ""))
			os.Exit(0)
		default:
			cmd.Help()
			os.Exit(0)
		}
	},
}

func rootUsageFunc(rootCmd *cobra.Command) error {
	bold := color.New(color.Bold)
	out := rootCmd.OutOrStderr()

	bold.Fprintf(out, "USAGE\n")
	fmt.Fprintf(out, "  %s [COMMAND] [FLAGS]\n", rootCmd.CommandPath())

	if rootCmd.HasAvailableLocalFlags() {
		bold.Fprintf(out, "\nFLAGS\n")
		fmt.Fprintf(out, rootCmd.LocalFlags().FlagUsages())
	}

	bold.Fprintf(out, "\nCOMMANDS\n")
	if rootCmd.HasAvailableSubCommands() {
		fmt.Fprintf(out, rootCommandUsages(rootCmd, "  "))
	} else {
		fmt.Fprintln(out, "  No commands found. Have you created a po.yml file?")
	}

	return nil
}

func init() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	rootCmd.SetUsageFunc(rootUsageFunc)
	rootCmd.Flags().BoolP("commands", "c", false, "list commands")
	rootCmd.Flags().BoolP("refresh", "", false, "clear import cache")

	config, err := loadAllConfigs()

	if err != nil {
		printError(rootCmd, err)
		os.Exit(2)
	}

	if config == nil {
		config = &Config{}
	}

	if err := buildCommandsFromConfig(config, rootCmd); err != nil {
		printError(rootCmd, err)
		os.Exit(3)
	}
}

func main() {
	if cmd, err := rootCmd.ExecuteC(); err != nil {
		printError(cmd, err)
		os.Exit(1)
	}
}
