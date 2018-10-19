package main

import (
	"crypto/sha1"
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Amount struct {
	AtLeastP *int `yaml:"at_least"`
	AtMostP  *int `yaml:"at_most"`
}

func (amount *Amount) AtLeast() int {
	if amount.AtLeastP == nil {
		return 1
	} else {
		return *amount.AtLeastP
	}
}

func (amount *Amount) AtMost() int {
	if amount.AtLeastP == nil && amount.AtMostP == nil {
		return 1
	} else if amount.AtMostP == nil {
		return 0
	} else {
		return *amount.AtMostP
	}
}

func (a *Amount) Merge(b *Amount) {
	if b.AtLeastP != nil {
		a.AtLeastP = b.AtLeastP
	}
	if b.AtMostP != nil {
		a.AtMostP = b.AtMostP
	}
}

type Argument struct {
	Var    string
	Short  string
	Amount Amount
}

func (a *Argument) Merge(b *Argument) {
	if b.Var != "" {
		a.Var = b.Var
	}
	if b.Short != "" {
		a.Short = b.Short
	}
	a.Amount.Merge(&b.Amount)
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
	Example  string
	Exec     string
	Script   string
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
	if b.Script != "" {
		a.Script = b.Script
	}

	if len(b.Args) > 0 {
		a.Args = b.Args
	}

	mergeFlags(a.Flags, b.Flags)
	mergeCommands(a.Commands, b.Commands)
}

var commandNameRegexp = regexp.MustCompile(`^\pL[\pL\d-_]*$`)

func validateCommandName(name string) error {
	if !commandNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid command name: %s", name)
	}
	return nil
}

func (command *Command) Validate() error {
	for name, subCommand := range command.Commands {
		if err := validateCommandName(name); err != nil {
			return err
		}
		if err := subCommand.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type Import struct {
	File string
	Url  string
}

func (imp *Import) Validate() error {
	if imp.File == "" && imp.Url == "" {
		return fmt.Errorf("import requires a 'url' or 'file' key set")
	}

	if imp.File != "" && imp.Url != "" {
		return fmt.Errorf("import cannot have both a 'url' and 'file' key set")
	}

	return nil
}

func mergeStringMaps(a map[string]string, b map[string]string) {
	for k, vb := range b {
		a[k] = vb
	}
}

type Config struct {
	Imports  []Import
	Aliases  map[string]string
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
		mergeStringMaps(a.Vars, b.Vars)
	}

	if a.Aliases == nil {
		a.Aliases = b.Aliases
	} else if b.Aliases != nil {
		mergeStringMaps(a.Aliases, b.Aliases)
	}
}

func (config *Config) Validate() error {
	for _, imp := range config.Imports {
		if err := imp.Validate(); err != nil {
			return err
		}
	}

	for name, _ := range config.Aliases {
		if err := validateCommandName(name); err != nil {
			return err
		}
	}

	for name, command := range config.Commands {
		if err := validateCommandName(name); err != nil {
			return err
		}
		if err := command.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func parseConfig(dat []byte) (*Config, error) {
	var config Config

	if err := yaml.Unmarshal(dat, &config); err != nil {
		return nil, err
	}

	return &config, config.Validate()
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

func sha1HexString(s string) string {
	h := sha1.New()
	h.Write([]byte(s))

	return fmt.Sprintf("%x", h.Sum(nil))
}

func readUrlCache(url string) ([]byte, error) {
	userCacheDir, err := os.UserCacheDir()

	if err != nil {
		return nil, err
	}

	cachePath := filepath.Join(userCacheDir, "po", "imports", sha1HexString(url))

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

	cacheDir := filepath.Join(userCacheDir, "po", "imports")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(cacheDir, sha1HexString(url))

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

const poPathEnvVar = "POPATH"
const poHomeEnvVar = "POHOME"

func loadAllConfigs() (*Config, error) {
	userCfgPath := userConfigPath()

	if err := os.Setenv(poHomeEnvVar, filepath.Dir(userCfgPath)); err != nil {
		return nil, err
	}

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

	if err := os.Setenv(poPathEnvVar, filepath.Dir(projectCfgPath)); err != nil {
		return nil, err
	}

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
		minLength += def.Amount.AtLeast()
	}

	return minLength
}

func maxArgLength(defs []Argument) int {
	maxLength := 0

	for _, def := range defs {
		if atMost := def.Amount.AtMost(); atMost == 0 {
			return -1
		} else {
			maxLength += atMost
		}
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
		required -= def.Amount.AtLeast()
		maxSlice := len(args) - required

		aNext := a

		if atMost := def.Amount.AtMost(); atMost == 0 {
			aNext += maxSlice
		} else {
			aNext += atMost
		}

		if aNext > maxSlice {
			aNext = maxSlice
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
		case maxLength > 0 && minLength == maxLength && len(args) != maxLength:
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

func buildScript(exec string, script string) string {
	return fmt.Sprintf("#! %s\n%s", exec, script)
}

func scriptCachePath(exec string, script string) (string, error) {
	userCacheDir, err := os.UserCacheDir()

	if err != nil {
		return "", err
	}

	cacheDir := filepath.Join(userCacheDir, "po", "scripts")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}

	scriptText := buildScript(exec, script)
	scriptPath := filepath.Join(cacheDir, sha1HexString(scriptText))

	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		err = ioutil.WriteFile(scriptPath, []byte(scriptText), 0755)
		return scriptPath, err
	}

	return scriptPath, nil
}

const defaultExecPath = "/bin/sh"

func execScript(exec string, env []string, script string) error {
	if exec == "" {
		exec = defaultExecPath
	}

	path, err := scriptCachePath(exec, script)

	if err != nil {
		return err
	}

	return unix.Exec(path, []string{}, env)
}

func formatArgDef(def Argument) string {
	arg := strings.ToUpper(def.Var)

	if def.Amount.AtLeast() > 1 || def.Amount.AtMost() != 1 {
		arg = fmt.Sprintf("%s...", arg)
	}

	if def.Amount.AtLeast() < 1 {
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

func getCommandAliases(config *Config, name string) []string {
	var aliases []string

	for k, v := range config.Aliases {
		if v == name {
			aliases = append(aliases, k)
		}
	}

	return aliases
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

func formatLines(format string, s string) string {
	lines := strings.Split(s, "\n")

	for i, line := range lines {
		lines[i] = fmt.Sprintf(format, line)
	}

	return strings.Join(lines, "")
}

func makeUsageFunc(parentCmd *cobra.Command, command *Command) func(*cobra.Command) error {
	bold := color.New(color.Bold)
	args := command.Args
	script := command.Script
	argUsageText := argUsages(command)

	return func(cobra *cobra.Command) error {
		out := cobra.OutOrStderr()

		if script != "" {
			bold.Fprintf(out, "USAGE\n")
			fmt.Fprintf(out, "  %s [FLAGS]\n", cobra.UseLine())

			if len(cobra.Aliases) > 0 {
				bold.Fprintf(out, "\nALIASES\n")
				fmt.Fprintf(out, "  %s\n", strings.Join(cobra.Aliases, ", "))
			}

			if len(args) > 0 {
				bold.Fprintf(out, "\nARGUMENTS\n")
				fmt.Fprintf(out, argUsageText)
			}

			if cobra.HasAvailableLocalFlags() {
				bold.Fprintf(out, "\nFLAGS\n")
				fmt.Fprintf(out, cobra.LocalFlags().FlagUsages())
			}

			if cobra.HasExample() {
				bold.Fprintf(out, "\nEXAMPLE\n")
				example := strings.TrimRight(cobra.Example, " \n")
				fmt.Fprintf(out, formatLines("  %s\n", example))
			}
		}

		if hasSubCommands(rootCmd, cobra) {
			if script != "" {
				fmt.Println()
			}

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
	if command.Script == "" {
		return nil
	}

	configEnv := configEnvVars(config)

	commandArgs := command.Args
	exec := command.Exec
	script := command.Script

	return func(cmd *cobra.Command, args []string) {
		env := os.Environ()
		env = append(env, configEnv...)
		env = append(env, argEnvVars(commandArgs, args)...)
		env = append(env, flagEnvVars(cmd.Flags())...)

		if err := execScript(exec, env, script); err != nil {
			log.Fatalf("error: %v", err)
		}
	}
}

func buildCommand(config *Config, parentCmd *cobra.Command, name string, command *Command) (*cobra.Command, error) {
	cmd := cobra.Command{
		Use:                   formatUsage(name, command),
		Aliases:               getCommandAliases(config, name),
		Short:                 command.Short,
		Long:                  command.Long,
		Args:                  argsMatchDefs(command.Args),
		Example:               command.Example,
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

	importsCacheDir := filepath.Join(userCacheDir, "po", "imports")

	if _, err := os.Stat(importsCacheDir); os.IsNotExist(err) {
		return nil
	}

	if err := deleteFilesInDir(importsCacheDir); err != nil {
		return err
	}

	scriptsCacheDir := filepath.Join(userCacheDir, "po", "scripts")

	if _, err := os.Stat(scriptsCacheDir); os.IsNotExist(err) {
		return nil
	}

	return deleteFilesInDir(scriptsCacheDir)
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
