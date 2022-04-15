package main

import (
	"os"
	"os/signal"

	derodpkg "github.com/civilware/derodpkg/cmd"
)

func main() {
	initparams := make(map[string]interface{})

	// Define all input params for derod - need to be sure most/all are the default from standard command line parser as we are not using that here
	initparams["--rpc-bind"] = "127.0.0.1:20202"
	initparams["--p2p-bind"] = "127.0.0.1:20201"
	initparams["--getwork-bind"] = "127.0.0.1:20200"

	chain := derodpkg.InitializeDerod(initparams)
	rpcserver := derodpkg.StartDerod(chain)

	var gracefulStop = make(chan os.Signal, 1)
	signal.Notify(gracefulStop, os.Interrupt) // listen to all signals
	for {
		sig := <-gracefulStop
		if sig.String() == "interrupt" {
			derodpkg.StopDerod(rpcserver, chain)
			break
		}
	}
}
