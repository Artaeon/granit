package main

import (
	"fmt"
	"os"
)

// runCompletion handles "granit completion <shell>" — outputs shell completion scripts.
func runCompletion(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: granit completion <bash|zsh|fish>")
		fmt.Println()
		fmt.Println("Generate shell completion scripts:")
		fmt.Println("  Bash:  granit completion bash  >> ~/.bashrc")
		fmt.Println("  Zsh:   granit completion zsh   >> ~/.zshrc")
		fmt.Println("  Fish:  granit completion fish   > ~/.config/fish/completions/granit.fish")
		os.Exit(1)
	}

	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		exitError("Unknown shell: %s (supported: bash, zsh, fish)", args[0])
	}
}

const bashCompletion = `# granit bash completion
_granit_completions() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    commands="open init daily today review sync scan serve export import backup plugin list capture clip query config todo search completion version help man"

    case "${prev}" in
        granit)
            COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
            # Also complete directories
            COMPREPLY+=( $(compgen -d -- "${cur}") )
            return 0
            ;;
        open|scan|daily|today|review|sync|serve|export|backup)
            # Complete directories for vault paths
            COMPREPLY=( $(compgen -d -- "${cur}") )
            return 0
            ;;
        import)
            COMPREPLY=( $(compgen -W "--from" -- "${cur}") )
            return 0
            ;;
        --from)
            COMPREPLY=( $(compgen -W "obsidian logseq notion" -- "${cur}") )
            return 0
            ;;
        plugin)
            COMPREPLY=( $(compgen -W "list install remove enable disable info create" -- "${cur}") )
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            return 0
            ;;
        todo)
            COMPREPLY=( $(compgen -W "--due --priority --tag --file --vault" -- "${cur}") )
            return 0
            ;;
        --priority)
            COMPREPLY=( $(compgen -W "highest high medium low" -- "${cur}") )
            return 0
            ;;
        --due)
            COMPREPLY=( $(compgen -W "today tomorrow monday tuesday wednesday thursday friday saturday sunday" -- "${cur}") )
            return 0
            ;;
        --format)
            COMPREPLY=( $(compgen -W "html text json" -- "${cur}") )
            return 0
            ;;
        review)
            COMPREPLY=( $(compgen -W "--week --markdown --md --save" -- "${cur}") )
            COMPREPLY+=( $(compgen -d -- "${cur}") )
            return 0
            ;;
        today)
            COMPREPLY=( $(compgen -W "--json" -- "${cur}") )
            COMPREPLY+=( $(compgen -d -- "${cur}") )
            return 0
            ;;
        sync)
            COMPREPLY=( $(compgen -W "--quiet --dry-run --message" -- "${cur}") )
            COMPREPLY+=( $(compgen -d -- "${cur}") )
            return 0
            ;;
    esac

    # Complete flags for any position
    if [[ "${cur}" == -* ]]; then
        local flags="--json --quiet --dry-run --markdown --md --save --week --help --version"
        COMPREPLY=( $(compgen -W "${flags}" -- "${cur}") )
        return 0
    fi

    # Default: directory completion
    COMPREPLY=( $(compgen -d -- "${cur}") )
}

complete -F _granit_completions granit
`

const zshCompletion = `#compdef granit

# granit zsh completion

_granit() {
    local -a commands
    commands=(
        'open:Open a vault in the TUI'
        'init:Initialize a new vault'
        'daily:Open or create today'\''s daily note'
        'today:Print today'\''s dashboard'
        'review:Generate daily or weekly review'
        'sync:Pull, commit, push in one command'
        'todo:Add a task to Tasks.md'
        'scan:Scan a vault and print statistics'
        'serve:Serve vault as read-only website'
        'search:Search vault content'
        'export:Export vault notes'
        'import:Import from Obsidian, Logseq, or Notion'
        'backup:Create a timestamped backup'
        'plugin:Manage plugins'
        'list:List vault notes or vaults'
        'capture:Quick-capture to inbox.md'
        'clip:Capture from stdin'
        'query:Query notes by metadata'
        'config:Show configuration'
        'completion:Generate shell completions'
        'version:Print version'
        'help:Show help'
        'man:Output man page'
    )

    _arguments -C \
        '1:command:->command' \
        '*::arg:->args'

    case $state in
        command)
            _describe -t commands 'granit commands' commands
            _files -/
            ;;
        args)
            case $words[1] in
                open|scan|daily|today|review|sync|serve|export|backup)
                    _files -/
                    ;;
                plugin)
                    local -a plugin_cmds
                    plugin_cmds=(list install remove enable disable info create)
                    _describe 'plugin command' plugin_cmds
                    ;;
                completion)
                    local -a shells
                    shells=(bash zsh fish)
                    _describe 'shell' shells
                    ;;
                import)
                    _arguments \
                        '--from[Source format]:format:(obsidian logseq notion)' \
                        '*:path:_files -/'
                    ;;
                todo)
                    _arguments \
                        '--due[Due date]:date:(today tomorrow monday tuesday wednesday thursday friday saturday sunday)' \
                        '--priority[Priority level]:priority:(highest high medium low)' \
                        '--tag[Add tag]:tag:' \
                        '--file[Target file]:file:_files' \
                        '--vault[Vault path]:vault:_files -/' \
                        '*:text:'
                    ;;
                review)
                    _arguments \
                        '--week[Weekly review]' \
                        '--markdown[Markdown output]' \
                        '--md[Markdown output]' \
                        '--save[Save to Reviews folder]' \
                        '*:path:_files -/'
                    ;;
                today)
                    _arguments \
                        '--json[JSON output]' \
                        '*:path:_files -/'
                    ;;
                sync)
                    _arguments \
                        '--quiet[Suppress output]' \
                        '-q[Suppress output]' \
                        '--dry-run[Show what would happen]' \
                        '-m[Commit message]:message:' \
                        '--message[Commit message]:message:' \
                        '*:path:_files -/'
                    ;;
                search)
                    _arguments \
                        '--json[JSON output]' \
                        '--regex[Regex search]' \
                        '--case-sensitive[Case-sensitive search]' \
                        '--no-color[Disable colors]' \
                        '*:query:'
                    ;;
                export)
                    _arguments \
                        '--format[Output format]:format:(html text json)' \
                        '--all[Export all notes]' \
                        '--note[Single note]:note:' \
                        '--output[Output directory]:dir:_files -/' \
                        '*:path:_files -/'
                    ;;
                list)
                    _arguments \
                        '--json[JSON output]' \
                        '--paths[Output paths only]' \
                        '--tags[List tags]' \
                        '--vaults[List vaults]' \
                        '*:path:_files -/'
                    ;;
            esac
            ;;
    esac
}

_granit "$@"
`

const fishCompletion = `# granit fish completion

# Disable file completion by default
complete -c granit -f

# Main commands
complete -c granit -n '__fish_use_subcommand' -a 'open' -d 'Open a vault in the TUI'
complete -c granit -n '__fish_use_subcommand' -a 'init' -d 'Initialize a new vault'
complete -c granit -n '__fish_use_subcommand' -a 'daily' -d 'Open or create today'\''s daily note'
complete -c granit -n '__fish_use_subcommand' -a 'today' -d 'Print today'\''s dashboard'
complete -c granit -n '__fish_use_subcommand' -a 'review' -d 'Generate daily or weekly review'
complete -c granit -n '__fish_use_subcommand' -a 'sync' -d 'Pull, commit, push in one command'
complete -c granit -n '__fish_use_subcommand' -a 'todo' -d 'Add a task to Tasks.md'
complete -c granit -n '__fish_use_subcommand' -a 'scan' -d 'Scan a vault and print statistics'
complete -c granit -n '__fish_use_subcommand' -a 'serve' -d 'Serve vault as read-only website'
complete -c granit -n '__fish_use_subcommand' -a 'search' -d 'Search vault content'
complete -c granit -n '__fish_use_subcommand' -a 'export' -d 'Export vault notes'
complete -c granit -n '__fish_use_subcommand' -a 'import' -d 'Import from Obsidian, Logseq, or Notion'
complete -c granit -n '__fish_use_subcommand' -a 'backup' -d 'Create a timestamped backup'
complete -c granit -n '__fish_use_subcommand' -a 'plugin' -d 'Manage plugins'
complete -c granit -n '__fish_use_subcommand' -a 'list' -d 'List vault notes or vaults'
complete -c granit -n '__fish_use_subcommand' -a 'capture' -d 'Quick-capture to inbox.md'
complete -c granit -n '__fish_use_subcommand' -a 'clip' -d 'Capture from stdin'
complete -c granit -n '__fish_use_subcommand' -a 'query' -d 'Query notes by metadata'
complete -c granit -n '__fish_use_subcommand' -a 'config' -d 'Show configuration'
complete -c granit -n '__fish_use_subcommand' -a 'completion' -d 'Generate shell completions'
complete -c granit -n '__fish_use_subcommand' -a 'version' -d 'Print version'
complete -c granit -n '__fish_use_subcommand' -a 'help' -d 'Show help'
complete -c granit -n '__fish_use_subcommand' -a 'man' -d 'Output man page'

# Enable directory completion for vault path commands
complete -c granit -n '__fish_seen_subcommand_from open scan daily today review sync serve export backup' -F

# plugin subcommands
complete -c granit -n '__fish_seen_subcommand_from plugin' -a 'list install remove enable disable info create'

# completion subcommands
complete -c granit -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'

# import flags
complete -c granit -n '__fish_seen_subcommand_from import' -l from -x -a 'obsidian logseq notion'

# todo flags
complete -c granit -n '__fish_seen_subcommand_from todo' -l due -x -a 'today tomorrow monday tuesday wednesday thursday friday saturday sunday'
complete -c granit -n '__fish_seen_subcommand_from todo' -l priority -x -a 'highest high medium low'
complete -c granit -n '__fish_seen_subcommand_from todo' -l tag -x
complete -c granit -n '__fish_seen_subcommand_from todo' -l file -r
complete -c granit -n '__fish_seen_subcommand_from todo' -l vault -r

# review flags
complete -c granit -n '__fish_seen_subcommand_from review' -l week -d 'Weekly review'
complete -c granit -n '__fish_seen_subcommand_from review' -s w -d 'Weekly review'
complete -c granit -n '__fish_seen_subcommand_from review' -l markdown -d 'Markdown output'
complete -c granit -n '__fish_seen_subcommand_from review' -l md -d 'Markdown output'
complete -c granit -n '__fish_seen_subcommand_from review' -l save -d 'Save to Reviews folder'

# today flags
complete -c granit -n '__fish_seen_subcommand_from today' -l json -d 'JSON output'

# sync flags
complete -c granit -n '__fish_seen_subcommand_from sync' -l quiet -d 'Suppress output'
complete -c granit -n '__fish_seen_subcommand_from sync' -s q -d 'Suppress output'
complete -c granit -n '__fish_seen_subcommand_from sync' -l dry-run -d 'Show what would happen'
complete -c granit -n '__fish_seen_subcommand_from sync' -s m -x -d 'Commit message'
complete -c granit -n '__fish_seen_subcommand_from sync' -l message -x -d 'Commit message'

# search flags
complete -c granit -n '__fish_seen_subcommand_from search' -l json -d 'JSON output'
complete -c granit -n '__fish_seen_subcommand_from search' -l regex -d 'Regex search'
complete -c granit -n '__fish_seen_subcommand_from search' -l case-sensitive -d 'Case-sensitive'
complete -c granit -n '__fish_seen_subcommand_from search' -l no-color -d 'Disable colors'

# export flags
complete -c granit -n '__fish_seen_subcommand_from export' -l format -x -a 'html text json'
complete -c granit -n '__fish_seen_subcommand_from export' -l all -d 'Export all notes'
complete -c granit -n '__fish_seen_subcommand_from export' -l note -x -d 'Single note'
complete -c granit -n '__fish_seen_subcommand_from export' -l output -r -d 'Output directory'

# list flags
complete -c granit -n '__fish_seen_subcommand_from list' -l json -d 'JSON output'
complete -c granit -n '__fish_seen_subcommand_from list' -l paths -d 'Output paths only'
complete -c granit -n '__fish_seen_subcommand_from list' -l tags -d 'List tags'
complete -c granit -n '__fish_seen_subcommand_from list' -l vaults -d 'List vaults'
`
