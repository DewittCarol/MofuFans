package main

import (
	"flag"
	"fmt"
	"io"
	"mofufans/internal/guard"
	"path/filepath"
)

func securityCLI(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("security", flag.ContinueOnError)
	flags.SetOutput(out)
	directory := flags.String("state-directory", "./data", "instance state_directory (stop the API first)")
	ip := flags.String("unban-ip", "", "remove bans for exactly this IPv4/IPv6 address")
	id := flags.String("unban-id", "", "remove one ban ID")
	all := flags.Bool("clear-bans", false, "remove all application bans and failed-attempt counters")
	if err := flags.Parse(args); err != nil {
		return err
	}
	choices := 0
	if *all {
		choices++
	}
	if *ip != "" {
		choices++
	}
	if *id != "" {
		choices++
	}
	if flags.NArg() != 0 || choices != 1 {
		return fmt.Errorf("use exactly one of --unban-ip, --unban-id or --clear-bans after stopping the API")
	}
	n, err := guard.Recover(filepath.Join(*directory, "security"), *ip, *id, *all)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Removed %d application ban(s). Restart the API to resume service.\n", n)
	return err
}
