package derodpkg

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/chzyer/readline"
	"github.com/docopt/docopt-go"

	"github.com/deroproject/derohe/block"
	"github.com/deroproject/derohe/blockchain"
	derodrpc "github.com/deroproject/derohe/cmd/derod/rpc"
	"github.com/deroproject/derohe/config"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/p2p"

	"github.com/go-logr/logr"
	"gopkg.in/natefinch/lumberjack.v2"
)

var command_line string = `derod 
DERO : A secure, private blockchain with smart-contracts

Usage:
  derod [--help] [--version] [--testnet] [--debug]  [--sync-node] [--timeisinsync] [--fastsync] [--socks-proxy=<socks_ip:port>] [--data-dir=<directory>] [--p2p-bind=<0.0.0.0:18089>] [--add-exclusive-node=<ip:port>]... [--add-priority-node=<ip:port>]... 	 [--min-peers=<11>] [--rpc-bind=<127.0.0.1:9999>] [--getwork-bind=<0.0.0.0:18089>] [--node-tag=<unique name>] [--prune-history=<50>] [--integrator-address=<address>] [--clog-level=1] [--flog-level=1]
  derod -h | --help
  derod --version

Options:
  -h --help     Show this screen.
  --version     Show version.
  --testnet  	Run in testnet mode.
  --debug       Debug mode enabled, print more log messages
  --clog-level=1	Set console log level (0 to 127) 
  --flog-level=1	Set file log level (0 to 127)
  --fastsync      Fast sync mode (this option has effect only while bootstrapping)
  --timeisinsync  Confirms to daemon that time is in sync, so daemon doesn't try to sync
  --socks-proxy=<socks_ip:port>  Use a proxy to connect to network.
  --data-dir=<directory>    Store blockchain data at this location
  --rpc-bind=<127.0.0.1:9999>    RPC listens on this ip:port
  --p2p-bind=<0.0.0.0:18089>    p2p server listens on this ip:port, specify port 0 to disable listening server
  --getwork-bind=<0.0.0.0:10100>    getwork server listens on this ip:port, specify port 0 to disable listening server
  --add-exclusive-node=<ip:port>	Connect to specific peer only 
  --add-priority-node=<ip:port>	Maintain persistant connection to specified peer
  --sync-node       Sync node automatically with the seeds nodes. This option is for rare use.
  --node-tag=<unique name>	Unique name of node, visible to everyone
  --integrator-address	if this node mines a block,Integrator rewards will be given to address.default is dev's address.
  --prune-history=<50>	prunes blockchain history until the specific topo_height
`

var Exit_In_Progress = make(chan bool)
var gracefulStop = make(chan os.Signal, 1)

var l *readline.Instance

// Do we need both? What purpose if this is solely derod and not for overall logging in chain software
var logger logr.Logger

// global logger all components will use it with context
var Logger logr.Logger = logr.Discard() // default discard all logs

var completer = readline.NewPrefixCompleter()

var params = map[string]interface{}{}

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

func InitializeDerod(initparams map[string]interface{}) (chain *blockchain.Blockchain) {
	runtime.MemProfileRate = 0
	var err error

	globals.Arguments, err = docopt.Parse(command_line, nil, true, config.Version.String(), false)

	// Default testnet to false if it is not defined, else initnetwork cannot be ran within globals.Initialize()
	if initparams["--testnet"] == nil {
		initparams["--testnet"] = false
	}

	for k, v := range initparams {
		globals.Arguments[k] = v
	}

	// We need to initialize readline first, so it changes stderr to ansi processor on windows
	l, err = readline.NewEx(&readline.Config{
		//Prompt:          "\033[92mDERO:\033[32m»\033[0m",
		Prompt:          "\033[92mDERO:\033[32m>>>\033[0m ",
		HistoryFile:     filepath.Join(os.TempDir(), "derod_readline.tmp"),
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})
	if err != nil {
		fmt.Printf("Error starting readline err: %s\n", err)
		return
	}
	defer l.Close()

	var network string
	switch globals.IsMainnet() {
	case false:
		network = "testnet"
	default:
		network = "mainnet"
	}

	exename, _ := os.Executable()
	globals.InitializeLog(l.Stdout(), &lumberjack.Logger{
		Filename:   exename + "_daemon_" + network + ".log",
		MaxSize:    100, // megabytes
		MaxBackups: 2,
	})

	logger = Logger.WithName("derod")

	logger.Info("DERO HE daemon :  It is an alpha version, use it for testing/evaluations purpose only.")
	logger.Info("Copyright 2017-2021 DERO Project. All rights reserved.")
	logger.Info("", "OS", runtime.GOOS, "ARCH", runtime.GOARCH, "GOMAXPROCS", runtime.GOMAXPROCS(0))
	logger.Info("", "Version", config.Version.String())

	logger.V(1).Info("", "Arguments", globals.Arguments)

	globals.Initialize() // setup network and proxy

	logger.V(0).Info("", "MODE", globals.Config.Name)
	logger.V(0).Info("", "Daemon data directory", globals.GetDataDirectory())

	// check  whether we are pruning, if requested do so
	prune_topo := int64(50)
	if _, ok := globals.Arguments["--prune-history"]; ok && globals.Arguments["--prune-history"] != nil { // user specified a limit, use it if possible
		i, err := strconv.ParseInt(globals.Arguments["--prune-history"].(string), 10, 64)
		if err != nil {
			logger.Error(err, "error Parsing --prune-history ")
			return
		} else {
			if i <= 1 {
				logger.Error(fmt.Errorf("--prune-history should be positive and more than 1"), "invalid argument")
				return
			} else {
				prune_topo = i
			}
		}
		logger.Info("will prune history till", "topo_height", prune_topo)

		if err := blockchain.Prune_Blockchain(prune_topo); err != nil {
			logger.Error(err, "Error pruning blockchain ")
			return
		} else {
			logger.Info("blockchain pruning successful")

		}
	}

	if _, ok := globals.Arguments["--timeisinsync"]; ok {
		globals.TimeIsInSync = globals.Arguments["--timeisinsync"].(bool)
	}

	if _, ok := globals.Arguments["--integrator-address"]; ok {
		params["--integrator-address"] = globals.Arguments["--integrator-address"]
	}

	chain, err = blockchain.Blockchain_Start(params)
	if err != nil {
		logger.Error(err, "Error starting blockchain")
		return
	}

	params["chain"] = chain

	// since user is using a proxy, he definitely does not want to give out his IP
	if globals.Arguments["--socks-proxy"] != nil {
		globals.Arguments["--p2p-bind"] = ":0"
		logger.Info("Disabling P2P server since we are using socks proxy")
	}

	return
}

func StartDerod(chain *blockchain.Blockchain) (rpcserver *derodrpc.RPCServer) {
	p2p.P2P_Init(params)
	rpcserver, _ = derodrpc.RPCServer_Start(params)

	go derodrpc.Getwork_server()

	// setup function pointers
	chain.P2P_Block_Relayer = func(cbl *block.Complete_Block, peerid uint64) {
		p2p.Broadcast_Block(cbl, peerid)
	}

	chain.P2P_MiniBlock_Relayer = func(mbl block.MiniBlock, peerid uint64) {
		p2p.Broadcast_MiniBlock(mbl, peerid)
	}

	{
		current_blid, err := chain.Load_Block_Topological_order_at_index(17600)
		if err == nil {

			current_blid := current_blid
			for {
				height := chain.Load_Height_for_BL_ID(current_blid)

				if height < 17500 {
					break
				}

				r, err := chain.Store.Topo_store.Read(int64(height))
				if err != nil {
					panic(err)
				}
				if r.BLOCK_ID != current_blid {
					fmt.Printf("Fixing corruption r %+v  , current_blid %s current_blid_height %d\n", r, current_blid, height)

					fix_commit_version, err := chain.ReadBlockSnapshotVersion(current_blid)
					if err != nil {
						panic(err)
					}

					chain.Store.Topo_store.Write(int64(height), current_blid, fix_commit_version, int64(height))

				}

				fix_bl, err := chain.Load_BL_FROM_ID(current_blid)
				if err != nil {
					panic(err)
				}
				current_blid = fix_bl.Tips[0]
			}
		}
	}
	globals.Cron.Start() // start cron jobs

	// This tiny goroutine continuously updates status as required
	go func() {
		for {
			// Must keep miner count - getwork server uses miner count value to loop through and send jobs
			derodrpc.CountMiners()
			time.Sleep(1 * time.Second)
		}
	}()

	setPasswordCfg := l.GenPasswordConfig()
	setPasswordCfg.SetListener(func(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
		l.SetPrompt(fmt.Sprintf("Enter password(%v): ", len(line)))
		l.Refresh()
		return nil, 0, false
	})
	l.Refresh() // refresh the prompt

	return
}

func StopDerod(rpcserver *derodrpc.RPCServer, chain *blockchain.Blockchain) {
	logger.Info("Exit in Progress, Please wait")
	time.Sleep(100 * time.Millisecond) // give prompt update time to finish

	rpcserver.RPCServer_Stop()
	p2p.P2P_Shutdown() // shutdown p2p subsystem
	chain.Shutdown()   // shutdown chain subsysem

	for globals.Subsystem_Active > 0 {
		logger.Info("Exit in Progress, Please wait.", "active subsystems", globals.Subsystem_Active)
		time.Sleep(1000 * time.Millisecond)
	}
}
