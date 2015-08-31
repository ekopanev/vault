package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/vault"
)

// MountCommand is a Command that mounts a new mount.
type MountCommand struct {
	Meta
}

func (c *MountCommand) Run(args []string) int {
	var description, path, defaultLeaseTTL, maxLeaseTTL string
	flags := c.Meta.FlagSet("mount", FlagSetDefault)
	flags.StringVar(&description, "description", "", "")
	flags.StringVar(&path, "path", "", "")
	flags.StringVar(&defaultLeaseTTL, "default_lease_ttl", "", "")
	flags.StringVar(&maxLeaseTTL, "max_lease_ttl", "", "")
	flags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := flags.Parse(args); err != nil {
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		flags.Usage()
		c.Ui.Error(fmt.Sprintf(
			"\nMount expects one argument: the type to mount."))
		return 1
	}

	mountType := args[0]

	// If no path is specified, we default the path to the backend type
	if path == "" {
		path = mountType
	}

	client, err := c.Client()
	if err != nil {
		c.Ui.Error(fmt.Sprintf(
			"Error initializing client: %s", err))
		return 2
	}

	mountInfo := &api.Mount{
		Type:        mountType,
		Description: description,
		Config:      &vault.MountConfig{},
	}

	var passConfig bool
	if defaultLeaseTTL != "" {
		mountInfo.Config.DefaultLeaseTTL, err = time.ParseDuration(defaultLeaseTTL)
		if err != nil {
			c.Ui.Error(fmt.Sprintf(
				"Error parsing default lease TTL duration: %s", err))
			return 2
		}
		passConfig = true
	}
	if maxLeaseTTL != "" {
		mountInfo.Config.MaxLeaseTTL, err = time.ParseDuration(maxLeaseTTL)
		if err != nil {
			c.Ui.Error(fmt.Sprintf(
				"Error parsing max lease TTL duration: %s", err))
			return 2
		}
		passConfig = true
	}

	if !passConfig {
		mountInfo.Config = nil
	}

	if err := client.Sys().Mount(path, mountInfo); err != nil {
		c.Ui.Error(fmt.Sprintf(
			"Mount error: %s", err))
		return 2
	}

	c.Ui.Output(fmt.Sprintf(
		"Successfully mounted '%s' at '%s'!",
		mountType, path))

	return 0
}

func (c *MountCommand) Synopsis() string {
	return "Mount a logical backend"
}

func (c *MountCommand) Help() string {
	helpText := `
Usage: vault mount [options] type

  Mount a logical backend.

  This command mounts a logical backend for storing and/or generating
  secrets.

General Options:

  ` + generalOptionsUsage() + `

Mount Options:

  -description=<desc>     Human-friendly description of the purpose for the
                          mount. This shows up in the mounts command.

  -path=<path>            Mount point for the logical backend. This defaults
                          to the type of the mount.

`
	return strings.TrimSpace(helpText)
}
