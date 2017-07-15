// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//---------------------------------------------------------------------------------------
// command line interface to developing and testing holochain applications

package main

import (
	"errors"
	"fmt"
	holo "github.com/metacurrency/holochain"
	"github.com/metacurrency/holochain/cmd"
	"github.com/metacurrency/holochain/ui"
	"github.com/urfave/cli"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"time"
)

const (
	defaultPort = "4141"
)

var debug, appInitialized bool
var rootPath, devPath, name string

func setupApp() (app *cli.App) {
	app = cli.NewApp()
	app.Name = "hcdev"
	app.Usage = "holochain dev command line tool"
	app.Version = fmt.Sprintf("0.0.0 (holochain %s)", holo.VersionStr)

	var service *holo.Service

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:        "debug",
			Usage:       "debugging output",
			Destination: &debug,
		},
		cli.StringFlag{
			Name:        "execpath",
			Usage:       "path to holochain dev execution directory (default: ~/.holochaindev)",
			Destination: &rootPath,
		},
		cli.StringFlag{
			Name:        "path",
			Usage:       "path to chain source definition directory (default: current working dir)",
			Destination: &devPath,
		},
	}

	var interactive bool
	var clonePath, scaffoldPath string
	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "initialize a holochain app directory: interactively, from a scaffold file or clone from another app",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:        "interactive",
					Usage:       "interactive initialization",
					Destination: &interactive,
				},
				cli.StringFlag{
					Name:        "clone",
					Usage:       "path to directory from which to clone the app",
					Destination: &clonePath,
				},
				cli.StringFlag{
					Name:        "scaffold",
					Usage:       "path to a scaffold file from which to initialize the app",
					Destination: &scaffoldPath,
				},
			},
			ArgsUsage: "<name>",
			Action: func(c *cli.Context) error {
				if appInitialized {
					return errors.New("current directory is an initialized app, apps shouldn't be nested")
				}

				args := c.Args()
				if len(args) != 1 {
					return errors.New("init: expecting app name as single argument")
				}

				if (interactive && clonePath != "") || (interactive && scaffoldPath != "") || (clonePath != "" && scaffoldPath != "") {
					return errors.New("options are mutually exclusive, please choose just one.")
				}
				name := args[0]
				devPath = filepath.Join(devPath, name)
				if interactive {
					// make the directory and chdir into it

					// terminates go process
					cmd.ExecBinScript("holochain.app.init.interactive")
				} else if clonePath != "" {
					// build the app by cloning from another app
					info, err := os.Stat(clonePath)
					if err != nil {
						return err
					}
					if !info.Mode().IsDir() {
						return errors.New("expecting a directory to clone from")
					}

					// TODO this is the bogus dev agent, really it should be someone else
					agent, err := holo.LoadAgent(rootPath)
					if err != nil {
						return err
					}

					err = service.Clone(clonePath, devPath, agent, true)
					if err != nil {
						return err
					}

					fmt.Printf("cloning %s from %s\n", name, clonePath)
				} else if scaffoldPath != "" {
					// build the app from the scaffold
					info, err := os.Stat(scaffoldPath)
					if err != nil {
						return err
					}
					if !info.Mode().IsRegular() {
						return errors.New("expecting a scaffold file")
					}
					fmt.Printf("initializing from scaffold:%s\n", scaffoldPath)
					fmt.Printf("WARNING: NOT IMPLEMENTED\n")
				} else {
					// build empty app directory template
					err := os.MkdirAll(devPath, os.ModePerm)
					if err != nil {
						return err
					}
					err = os.MkdirAll(filepath.Join(devPath, holo.ChainDNADir), os.ModePerm)
					if err != nil {
						return err
					}
					err = os.MkdirAll(filepath.Join(devPath, holo.ChainUIDir), os.ModePerm)
					if err != nil {
						return err
					}
					err = os.MkdirAll(filepath.Join(devPath, holo.ChainTestDir), os.ModePerm)
					if err != nil {
						return err
					}
					fmt.Printf("initializing empty application template\n")
				}

				err := os.Chdir(devPath)
				if err != nil {
					return err
				}

				// finish by creating the .hc directory
				// terminates go process
				cmd.ExecBinScript("holochain.app.init", name, name)

				return nil
			},
		},
		{
			Name:      "test",
			Aliases:   []string{"t"},
			ArgsUsage: "no args run's all stand-alone | [test file prefix] | [scenario] [role]",
			Usage:     "run chain's stand-alone or scenario tests",
			Action: func(c *cli.Context) error {
				var err error
				var h *holo.Holochain
				h, err = getHolochain(c, service)
				if err != nil {
					return err
				}
				args := c.Args()
				var errs []error

				if len(args) == 2 {
					dir := h.TestPath() + "/" + args[0]
					role := args[1]

					err, errs = h.TestScenario(dir, role)
					if err != nil {
						return err
					}
				} else if len(args) == 1 {
					errs = h.TestOne(args[0])
				} else if len(args) == 0 {
					errs = h.Test()
				} else {
					return errors.New("test: expected 0 args (run all stand-alone tests), 1 arg (a single stand-alone test) or 2 args (scenario and role)")
				}

				var s string
				for _, e := range errs {
					s += e.Error()
				}
				if s != "" {
					return errors.New(s)
				}
				return nil
			},
		},
		{
			Name:      "scenario",
			Aliases:   []string{"s"},
			Usage:     "run a scenario test",
			ArgsUsage: "scenario-name",
			Action: func(c *cli.Context) error {
				if !appInitialized {
					return errors.New("please initialize this app with 'hcdev init'")
				}

				args := c.Args()
				if len(args) != 1 {
					return errors.New("missing scenario name argument")
				}

				// terminates go process
				cmd.ExecBinScript("holochain.app.testScenario", args[0])
				return nil
			},
		},
		{
			Name:      "web",
			Aliases:   []string{"serve", "w"},
			ArgsUsage: "[port]",
			Usage:     fmt.Sprintf("serve a chain to the web on localhost:<port> (defaults to %s)", defaultPort),
			Action: func(c *cli.Context) error {

				h, err := getHolochain(c, service)
				if err != nil {
					return err
				}
				h, err = service.GenChain(name)
				if err != nil {
					return err
				}

				var port string
				if len(c.Args()) == 0 {
					port = defaultPort
				} else {
					port = c.Args()[0]
				}
				fmt.Printf("Serving holochain with DNA hash:%v on port:%s\n", h.DNAHash(), port)

				err = h.Activate()
				if err != nil {
					return err
				}
				//				go h.DHT().HandleChangeReqs()
				go h.DHT().HandleGossipWiths()
				go h.DHT().Gossip(2 * time.Second)
				ui.NewWebServer(h, port).Start()
				return err
			},
		}}

	app.Before = func(c *cli.Context) error {
		if debug {
			os.Setenv("DEBUG", "1")
		}
		holo.Initialize()

		var err error
		if devPath == "" {
			devPath, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		name = path.Base(devPath)

		if cmd.IsAppDir(devPath) == nil {
			appInitialized = true
		}

		if rootPath == "" {
			rootPath = os.Getenv("HOLOPATH")
			if rootPath == "" {
				u, err := user.Current()
				if err != nil {
					return err
				}
				userPath := u.HomeDir
				rootPath = userPath + "/" + holo.DefaultDirectoryName + "dev"
			}
		}
		if !holo.IsInitialized(rootPath) {
			service, err = holo.Init(rootPath, holo.AgentName("test@example.com"))
			if err != nil {
				return err
			}
			fmt.Println("Holochain dev service initialized:")
			fmt.Printf("    %s directory created\n", rootPath)
			fmt.Printf("    defaults stored to %s\n", holo.SysFileName)
			fmt.Println("    key-pair generated")
			fmt.Printf("    default agent stored to %s\n", holo.AgentFileName)

		} else {
			service, err = holo.LoadService(rootPath)
		}
		return err
	}

	app.Action = func(c *cli.Context) error {
		cli.ShowAppHelp(c)

		return nil
	}
	return

}

func main() {
	app := setupApp()

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func getHolochain(c *cli.Context, service *holo.Service) (h *holo.Holochain, err error) {
	fmt.Printf("Copying chain to: %s\n", rootPath)
	err = os.RemoveAll(rootPath + "/" + name)
	if err != nil {
		return
	}
	var agent holo.Agent
	agent, err = holo.LoadAgent(rootPath)
	if err != nil {
		return
	}
	err = service.Clone(devPath, rootPath+"/"+name, agent, false)
	if err != nil {
		return
	}
	h, err = service.Load(name)
	if err != nil {
		return
	}
	return
}
