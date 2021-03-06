// Subutai binary is consist of two parts: Agent and CLI
//
// Both packages placed in relevant directories. Detailed explanation can be found in github Wiki page: https://github.com/subutai-io/snap/wiki
package main

import (
	"os"

	"github.com/subutai-io/agent/agent"
	"github.com/subutai-io/agent/cli"
	"github.com/subutai-io/agent/config"
	"github.com/subutai-io/agent/log"

	gcli "github.com/urfave/cli"
	"github.com/subutai-io/agent/lib/gpg"
	"github.com/subutai-io/agent/lib/exec"
	"strings"
	"github.com/subutai-io/agent/db"
)

var version = "unknown"

func init() {
	if os.Getuid() != 0 {
		log.Error("Please run as root")
	}

	checkGPG()
}

func checkGPG() {
	out, err := exec.Execute("gpg1", "--version")
	if err != nil {
		out, err = exec.Execute("gpg", "--version")

		if err != nil {
			log.Fatal("GPG not found " + out)
		} else {
			lines := strings.Split(out, "\n")
			if len(lines) > 0 && strings.HasPrefix(lines[0], "gpg (GnuPG) ") {
				version := strings.TrimSpace(strings.TrimPrefix(lines[0], "gpg (GnuPG)"))
				if strings.HasPrefix(version, "1.4") {
					gpg.GPG = "gpg"
				} else {
					log.Fatal("GPG version " + version + " is not compatible with subutai")
				}
			} else {
				log.Fatal("Failed to determine GPG version " + out)
			}
		}
	} else {
		lines := strings.Split(out, "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], "gpg (GnuPG) ") {
			version := strings.TrimSpace(strings.TrimPrefix(lines[0], "gpg (GnuPG)"))
			if strings.HasPrefix(version, "1.4") {
				gpg.GPG = "gpg1"
			} else {
				log.Fatal("GPG version " + version + " is not compatible with subutai")
			}
		} else {
			log.Fatal("Failed to determine GPG version " + out)
		}
	}
}

func loadManagementIp() {
	if len(strings.TrimSpace(config.Management.Host)) == 0 {
		ip, err := db.INSTANCE.DiscoveryLoad()
		if !log.Check(log.WarnLevel, "Loading discovered ip from db", err) {
			config.Management.Host = ip
		}
	}
}

func main() {
	app := gcli.NewApp()
	app.Name = "Subutai"
	app.Version = version
	app.Usage = "daemon and command line interface binary"

	if len(os.Args) > 1 && os.Args[len(os.Args)-1] != "daemon" {
		loadManagementIp()
	}

	app.Flags = []gcli.Flag{gcli.BoolFlag{
		Name:  "d",
		Usage: "debug mode"}}

	app.Before = func(c *gcli.Context) error {
		if c.IsSet("d") {
			log.Level(log.DebugLevel)
		}
		return nil
	}

	app.Commands = []gcli.Command{{
		Name: "attach", Usage: "attach to Subutai container",
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcAttach(c.Args().Get(0), c.Args().Tail())
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "batch", Usage: "batch commands execution",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "json, j", Usage: "JSON string with commands"}},
		Action: func(c *gcli.Context) error {
			if c.String("j") != "" {
				cli.Batch(c.String("j"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "clone", Usage: "clone Subutai container",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "env, e", Usage: "set environment id for container"},
			gcli.StringFlag{Name: "ipaddr, i", Usage: "set container IP address and VLAN"},
			gcli.StringFlag{Name: "token, t", Usage: "CDN token to clone private and shared templates"},
			gcli.StringFlag{Name: "secret, s", Usage: "Console secret"}},
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" && c.Args().Get(1) != "" {
				cli.LxcClone(c.Args().Get(0), c.Args().Get(1), c.String("e"), c.String("i"), c.String("s"), c.String("t"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "cleanup", Usage: "clean Subutai environment",
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcDestroy(c.Args().Get(0), true, false)
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "prune", Usage: "prune unused templates",
		Action: func(c *gcli.Context) error {
			cli.Prune()
			return nil
		}}, {

		Name: "config", Usage: "edit container config",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "operation, o", Usage: "<add|del> operation"},
			gcli.StringFlag{Name: "key, k", Usage: "configuration key"},
			gcli.StringFlag{Name: "value, v", Usage: "configuration value"},
		},
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcConfig(c.Args().Get(0), c.String("o"), c.String("k"), c.String("v"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "daemon", Usage: "start Subutai agent",
		Action: func(c *gcli.Context) error {
			config.InitAgentDebug()
			agent.Start()
			return nil
		}}, {

		Name: "destroy", Usage: "destroy Subutai container/template",
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcDestroy(c.Args().Get(0), false, false)
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "export", Usage: "export Subutai container",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "name, n", Usage: "new template name"},
			gcli.StringFlag{Name: "version, v", Usage: "template version"},
			gcli.StringFlag{Name: "size, s", Usage: "template preferred size"},
			gcli.StringFlag{Name: "token, t", Usage: "mandatory CDN token"},
			gcli.StringFlag{Name: "description, d", Usage: "template description"},
			gcli.BoolFlag{Name: "local, l", Usage: "export template to local cache"}},
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcExport(c.Args().Get(0), c.String("n"), c.String("v"), c.String("s"),
					c.String("t"), c.String("d"), c.Bool("l"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "import", Usage: "import Subutai template",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "token, t", Usage: "CDN token to import private and shared templates"},
			gcli.BoolFlag{Name: "local, l", Usage: "prefer to use local template archive"}},
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcImport(c.Args().Get(0), c.String("t"), c.Bool("l"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "info", Usage: "information about host system",
		Action: func(c *gcli.Context) error {
			cli.Info(c.Args().Get(0), c.Args().Get(1))
			return nil
		}}, {

		Name: "hostname", Usage: "Set hostname of container or host",
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) == "" {
				gcli.ShowSubcommandHelp(c)
			} else if len(c.Args().Get(1)) != 0 {
				cli.LxcHostname(c.Args().Get(0), c.Args().Get(1))
			} else {
				cli.Hostname(c.Args().Get(0))
			}
			return nil
		}}, {

		Name: "list", Usage: "list Subutai container",
		Flags: []gcli.Flag{
			gcli.BoolFlag{Name: "container, c", Usage: "containers only"},
			gcli.BoolFlag{Name: "template, t", Usage: "templates only"},
			gcli.BoolFlag{Name: "info, i", Usage: "detailed container info"},
			gcli.BoolFlag{Name: "parent, p", Usage: "with parent"}},
		Action: func(c *gcli.Context) error {
			cli.LxcList(c.Args().Get(0), c.Bool("c"), c.Bool("t"), c.Bool("i"), c.Bool("p"))
			return nil
		}}, {

		Name: "map", Usage: "Subutai port mapping",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "internal, i", Usage: "internal socket"},
			gcli.StringFlag{Name: "external, e", Usage: "RH port"},
			gcli.StringFlag{Name: "domain, d", Usage: "domain name"},
			gcli.StringFlag{Name: "cert, c", Usage: "https certificate"},
			gcli.StringFlag{Name: "policy, p", Usage: "balancing policy"},
			gcli.BoolFlag{Name: "list, l", Usage: "list mapped ports"},
			gcli.BoolFlag{Name: "remove, r", Usage: "remove map"},
			gcli.BoolFlag{Name: "sslbackend", Usage: "ssl backend in https upstream"},
		},
		Action: func(c *gcli.Context) error {
			if len(c.Args()) > 0 || c.NumFlags() > 0 {
				cli.MapPort(c.Args().Get(0), c.String("i"), c.String("e"), c.String("p"), c.String("d"), c.String("c"), c.Bool("l"), c.Bool("r"), c.Bool("sslbackend"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "metrics", Usage: "list Subutai container",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "start, s", Usage: "start time"},
			gcli.StringFlag{Name: "end, e", Usage: "end time"}},
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.HostMetrics(c.Args().Get(0), c.String("s"), c.String("e"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "proxy", Usage: "Subutai reverse proxy",
		Subcommands: []gcli.Command{
			{
				Name:     "add",
				Usage:    "add reverse proxy component",
				HideHelp: true,
				Flags: []gcli.Flag{
					gcli.StringFlag{Name: "domain, d", Usage: "add domain to vlan"},
					gcli.StringFlag{Name: "host, h", Usage: "add host to domain on vlan"},
					gcli.StringFlag{Name: "policy, p", Usage: "set load balance policy (rr|lb|hash)"},
					gcli.StringFlag{Name: "file, f", Usage: "specify pem certificate file"}},
				Action: func(c *gcli.Context) error {
					cli.ProxyAdd(c.Args().Get(0), c.String("d"), c.String("h"), c.String("p"), c.String("f"))
					return nil
				},
			},
			{
				Name:     "del",
				Usage:    "del reverse proxy component",
				HideHelp: true,
				Flags: []gcli.Flag{
					gcli.BoolFlag{Name: "domain, d", Usage: "delete domain from vlan"},
					gcli.StringFlag{Name: "host, h", Usage: "delete host from domain on vlan"}},
				Action: func(c *gcli.Context) error {
					cli.ProxyDel(c.Args().Get(0), c.String("h"), c.Bool("d"))
					return nil
				},
			},
			{
				Name:     "check",
				Usage:    "check existing domain or host",
				HideHelp: true,
				Flags: []gcli.Flag{
					gcli.BoolFlag{Name: "domain, d", Usage: "check domains on vlan"},
					gcli.StringFlag{Name: "host, h", Usage: "check hosts on vlan"}},
				Action: func(c *gcli.Context) error {
					cli.ProxyCheck(c.Args().Get(0), c.String("h"), c.Bool("d"))
					return nil
				},
			},
		}}, {

		Name: "quota", Usage: "set quotas for Subutai container",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "set, s", Usage: "set quota for the specified resource type (cpu, cpuset, ram, disk, network)"},
			gcli.StringFlag{Name: "threshold, t", Usage: "set alert threshold"}},
		Action: func(c *gcli.Context) error {
			cli.LxcQuota(c.Args().Get(0), c.Args().Get(1), c.String("s"), c.String("t"))
			return nil
		}}, {

		Name: "stats", Usage: "statistics from host",
		Action: func(c *gcli.Context) error {
			cli.Info(c.Args().Get(0), c.Args().Get(1))
			return nil
		}}, {

		Name: "start", Usage: "start Subutai container",
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcStart(c.Args().Get(0))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "stop", Usage: "stop Subutai container",
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcStop(c.Args().Get(0))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "restart", Usage: "restart Subutai container",
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.LxcRestart(c.Args().Get(0))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "tunnel", Usage: "SSH tunnel management",
		Subcommands: []gcli.Command{
			{
				Name:  "add",
				Usage: "add ssh tunnel",
				Flags: []gcli.Flag{
					gcli.BoolFlag{Name: "global, g", Usage: "create tunnel to global proxy"}},
				Action: func(c *gcli.Context) error {
					cli.TunAdd(c.Args().Get(0), c.Args().Get(1))
					return nil
				}}, {
				Name:  "del",
				Usage: "delete tunnel",
				Action: func(c *gcli.Context) error {
					cli.TunDel(c.Args().Get(0))
					return nil
				}}, {
				Name:  "list",
				Usage: "list active ssh tunnels",
				Action: func(c *gcli.Context) error {
					cli.TunList()
					return nil
				}}, {
				Name:  "check",
				Usage: "check active ssh tunnels",
				Action: func(c *gcli.Context) error {
					cli.TunCheck()
					return nil
				}},
		}}, {

		Name: "update", Usage: "update Subutai management, container or Resource host",
		Flags: []gcli.Flag{
			gcli.BoolFlag{Name: "check, c", Usage: "check for updates without installation"}},
		Action: func(c *gcli.Context) error {
			if c.Args().Get(0) != "" {
				cli.Update(c.Args().Get(0), c.Bool("c"))
			} else {
				gcli.ShowSubcommandHelp(c)
			}
			return nil
		}}, {

		Name: "vxlan", Usage: "VXLAN tunnels operation",
		Flags: []gcli.Flag{
			gcli.StringFlag{Name: "create, c", Usage: "create vxlan tunnel"},
			gcli.StringFlag{Name: "delete, d", Usage: "delete vxlan tunnel"},
			gcli.BoolFlag{Name: "list, l", Usage: "list vxlan tunnels"},

			gcli.StringFlag{Name: "remoteip, r", Usage: "vxlan tunnel remote ip"},
			gcli.StringFlag{Name: "vlan, vl", Usage: "tunnel vlan"},
			gcli.StringFlag{Name: "vni, v", Usage: "vxlan tunnel vni"},
		},
		Action: func(c *gcli.Context) error {
			cli.VxlanTunnel(c.String("c"), c.String("d"), c.String("r"), c.String("vl"), c.String("v"), c.Bool("l"))
			return nil
		}},
	}

	app.Run(os.Args)
}
