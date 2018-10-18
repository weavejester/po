# po

po is a command-line tool for organizing project-specific scripts.

**Disclaimer:** This tool is not yet stable, and the flags, arguments
and configuration syntax may change.

## Rationale

Software projects often accumulate scripts for performing various
tasks, such as running tests, migrating databases, and so forth.

Often these scripts will be either be individual shell scripts in a
`bin` directory, or they'll be called by a build tool like [make][],
[rake][], or whatever the project maintainer happens to prefer.

However, neither of these solutions is ideal. A directory of scripts
is not easily shared between projects, and a build tool is designed
for building and managing dependencies, rather than providing a good
interface to arbitrary scripts.

po attempts to solve these problems. Scripts are defined by a `po.yml`
file in your project directory. This file can reference local scripts,
or pull in scripts from URLs. Commands are discoverable, and flags and
arguments are handled intelligently.

[make]: https://www.gnu.org/software/make/
[rake]: https://github.com/ruby/rake


## Install

If you have [go][] installed, you can run:

    go get -u github.com/weavejester/po

This will install po in your `$GOPATH/bin` directory. If this is on
your `$PATH`, then you can start using `po` immediately.

[go]: https://golang.org/


## Usage

### Getting Started

A minimal `po.yml` file looks like:

```yaml
commands:
  hello:
    script: echo Hello World
```

As you might expect, if you run `po hello`, you get a message:

```
$ po hello
Hello World
```

If we just run `po`, we can see this command listed:

```
$ po
CLI for managing project-specific scripts

USAGE
  po [COMMAND] [FLAGS]

FLAGS
  -c, --commands   list commands
  -h, --help       help for po
      --refresh    clear import cache
      --version    version for po

COMMANDS
  hello
  help        Help about any command
```

We can also run `po --commands` for a more concise and
tooling-friendly list:

```
$ po --commands
hello
help        Help about any command
```

It would be nice if we could add a description to our `hello`
script, and we can do this by adding `short` and `long` keys to our
`po.yml` file:

```yaml
commands:
  hello:
    short: Prints a greeting
    long: Prints 'Hello World' to STDOUT.
    script: echo Hello World
```

Now when we take a look at the commands:

```
$ po --commands
hello       Prints a greeting
help        Help about any command
```

If we use `po help`, we can get a longer description:

```
$ po help hello
Prints 'Hello World' to STDOUT.

USAGE
  po hello [FLAGS]

FLAGS
  -h, --help   help for hello
```

We could also get the same message with `po hello --help` or `po hello
-h`.


### Arguments

Commands can be allowed additional arguments. Suppose we want to add
an argument to `po hello` called `name`:

```yaml
commands:
  hello:
    short: Prints a greeting
    long: Prints 'Hello NAME' to STDOUT.
    args:
      - var: name
        desc: a name to greet
    script: echo Hello $name
```

Now if we run `po hello --help` we can see the command has an
argument:

```
$ po hello --help
Prints 'Hello NAME' to STDOUT.

USAGE
  po hello NAME [FLAGS]

ARGUMENTS
  NAME

FLAGS
  -h, --help   help for hello
```

Calling `po hello` without an argument results in an error:

```
$ po help
ERROR [po hello]: requires exactly 1 arguments
Run 'po hello --help' for usage.
```

And calling it with an argument results in the output you might
expect:

```
$ po hello Bob
Hello Bob
```


### Flags

Flags are also supported, and are often more descriptive and flexible
than a bare argument. Let's modify our example to change the name
argument to a flag:

```yaml
commands:
  hello:
    short: Prints a greeting
    long: Prints 'Hello NAME' to STDOUT.
    flags:
      name:
        type: string
        desc: a name to greet
        short: n
        default: World
    script: echo Hello $name
```

If we take a look at the help for this command again:

```
Prints 'Hello NAME' to STDOUT.

USAGE
  po hello [FLAGS]

FLAGS
  -h, --help          help for hello
  -n, --name string   a name to greet (default "World")
```

This command now can be used in a few different ways:

```
$ po hello --name=Alice
Hello Alice

$ po hello --name Bob
Hello Bob

$ po hello -n Carol
Hello Carol

$ po hello
Hello World
```


### Vars

You've probably noticed that flags and arguments are passed to the run
scripts for the commands as environment variables. You can also define
static variables in the `vars` directive:

```yaml
vars:
  greet: Hey
commands:
  hello:
    short: Prints a greeting
    long: Prints 'Hello NAME' to STDOUT.
    flags:
      name:
        type: string
        desc: a name to greet
        short: n
        default: World
    script: echo $greet $name
```

Running the command:

```
$ po hello --name Bob
Hey Bob
```

Vars are particularly useful when they are defined in a user's
`po.yml` file, located at: `$HOME/.config/po/po.yml`. The
user-specific `po.yml` will be merged with the project's
`po.yml`, so you can define things like API keys in your user
configuration that the project configuration later uses.

Vars are also useful for customizing the behavior of imports.


### Imports

Imports allow you to merge in a commands and vars from an external
source. This can be a local file, or a URL.

For example, let's add an import to our `po.yml` file:

```yaml
imports:
  - url: https://git.io/fxVcZ
commands:
  hello:
    short: Prints a greeting
    long: Prints a greeting to STDOUT.
    flags:
      name:
        type: string
        desc: a name to greet
        default: World
    script: echo Hello $name
```

This import adds an extra command, `po bye`

```
$ po --commands
bye         Prints a farewell
hello       Prints a greeting
help        Help about any command
```

URL imports are cached locally indefinitely. To force po to clear its
cache and re-download imported URLs, run:

```
$ po --refresh
```


### Exec

You can change the interpreter for the script via the `exec`
option. By default scripts are executed with `/bin/sh`, but you can
change this to anything compatible with a shebang line:

```yaml
commands:
  hello:
    short: Prints a greeting
    long: Prints 'Hello NAME' to STDOUT.
    flags:
      name:
        type: string
        desc: a name to greet
        short: n
        default: World
    exec: /usr/bin/env python3
    script: |
      import os
      print("Hello", os.environ['name'])
```

Flags and arguments are still handled through environment variables.


### Nesting

Commands can be nested below other commands. We can use this to add an
alternative version of our `hello` command:

```yaml
commands:
  hello:
    short: Prints a greeting
    long: Prints 'Hello NAME' to STDOUT.
    flags:
      name:
        type: string
        desc: a name to greet
        short: n
        default: World
    script: echo Hello $name
    commands:
      loud:
        short: Loudly prints a greeting
        long: Prints a greeting in uppercase to STDOUT.
        flags:
          name:
            type: string
            desc: a name to greet
            short: n
            default: World
        script: |
          echo Hello $name | awk '{print toupper}'
```

If we check the help message for `hello`, we can see it now has a
subcommand, `hello:loud`:

```
$ po hello --help
Prints 'Hello NAME' to STDOUT.

USAGE
  po hello [FLAGS]

FLAGS
  -h, --help          help for hello
  -n, --name string   a name to greet (default "World")

COMMANDS
  hello:loud  Loudly prints a greeting
```

If we run `hello:loud`:

```
$ po hello:loud --name Alice
HELLO ALICE
```

Subcommands can be used to create alternative versions of existing
commands, or to group similar commands together. For example, you
might have a `db:migrate` and `db:seed` task.
