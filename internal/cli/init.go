package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
)

// configTemplate is the configuration file "cronx init" writes: the fields a new
// installation needs are set, and every other option cronx knows is written
// next to them commented out, with what it does and what happens without it, so
// that the file is also its own reference. The tests keep it honest: it is a
// valid configuration both as it stands and with every commented option turned
// on, and it documents every option the reference configuration uses.
const configTemplate = `# The configuration of cronx: the jobs to run, and the settings of the scheduler.
#
# The job below is the smallest useful one: its schedule, its command and the
# arguments of the command are set. Every other option is commented out, with
# what it does and what happens without it, so uncomment the lines you need. The
# complete reference is docs/configuration.md.
#
# The path of this file is resolved in this order: the --config flag, the
# CRONX_CONFIG environment variable, and then ~/.cronx/config.toml. cronx only
# ever reads it, and it refuses a key it does not know instead of ignoring it.
#
# A running scheduler reads this file again when it is sent SIGHUP: the jobs it
# holds take effect at once, while a change to [scheduler], [logging] or
# [storage] needs a restart.

[scheduler]
# timezone = "Local"            # an IANA name such as "Europe/Rome", or "Local" for the zone of the machine
# max_parallel_jobs = 1         # how many jobs may run at the same time

[logging]
# level = "info"                # debug | info | warn | error
# path = "/var/log/cronx/runs.log"   # where the output of every run goes (default: ~/.cronx/logs/runs.log)
# max_size = "10MB"             # rotate the run log once it would pass this size (default: never)
# max_backups = 3               # how many rotated run logs are kept

[storage]
# path = "/var/lib/cronx/state.db"   # the history and the runtime state (default: ~/.cronx/state.db)
# max_runs = 1000               # how many runs of every job the history keeps (default: all of them)

# The jobs. A job is one [jobs.<name>] table; the name may hold letters, digits,
# - and _. Its schedule is a five-field cron expression, or a descriptor:
# @yearly, @monthly, @weekly, @daily, @midnight and @hourly stand for a fixed
# time, and @reboot runs the job once, when the scheduler starts.
[jobs.hello]
schedule = "*/5 * * * *"        # a five-field cron expression: every five minutes
command = "/bin/echo"           # required, an absolute path: cronx never looks a command up in PATH
args = ["hello from cronx"]     # passed to the program as they are, never through a shell
# timeout = "30m"               # stop it, and the processes it started, after this long (default: no timeout)
# grace_period = "10s"          # how long it is given to stop after SIGTERM (default: 10s)
# retry = 1                     # further attempts after a failure, recorded one by one (default: none)
# overlap = "skip"              # skip | allow | queue: what a trigger does while a run is in progress
# working_directory = "/tmp"    # where the program runs (default: the directory of the scheduler)
# env = { TIER = "gold" }       # extra variables for the process, which inherits nothing else

# Add the jobs you need, pointing command at programs of your machine, then:
#
#   cronx validate   check the file
#   cronx list       see the jobs and when each one runs next
#   cronx run        start the scheduler
`

// newInitCommand builds the "init" command, which writes a configuration file to
// start from.
func newInitCommand(configPath *string) *cobra.Command {
	var force bool

	command := &cobra.Command{
		Use:   "init",
		Short: "Write a configuration file to start from",
		Long: "Write a configuration file at the path the configuration is looked for\n" +
			"in, creating the directory it lives in.\n\n" +
			"The fields one job needs are set, and every other option cronx knows is\n" +
			"written next to them commented out, with what it does and what happens\n" +
			"without it, so that the file can be read as its own reference.\n\n" +
			"A file that already exists is never replaced: the command stops instead,\n" +
			"names the file and leaves it untouched. --force replaces it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.ResolvePath(*configPath)
			if err != nil {
				return err
			}
			if err := writeConfiguration(path, force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s written\n", path)
			return nil
		},
	}

	command.Flags().BoolVar(&force, "force", false,
		"replace the configuration file if it already exists")
	return command
}

// writeConfiguration writes the configuration template at path, creating the
// directory it lives in. A file that is already there is only replaced when
// force is set.
func writeConfiguration(path string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating the directory of %s: %w", path, err)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s already exists: pass --force to replace it", path)
		}
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if _, err := file.WriteString(configTemplate); err != nil {
		_ = file.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
