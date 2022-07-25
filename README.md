# derodpkg
Importable package written in Golang to run DERO Daemon as a service within other applications/uses. Relies heavily on import cases from https://github.com/deroproject/derohe

# Using In Your Application/Service
Derod starts out with a subset of parameters. It is important to either surface a way to pass custom parameters or pre-define a list of parameters and their respective values. You can reference some options here: https://github.com/deroproject/derohe/blob/main/cmd/derod/main.go#L58

## Install
```
go install github.com/civilware/derodpkg
```

## Importing DERO Daemon Package
You can either just import directly to the /cmd directory or define a name to leverage such as derodpkg for the import directory

```go
import derodpkg "github.com/civilware/derodpkg/cmd"
```

## Initializing DERO Daemon

```go
initparams := make(map[string]interface{})

// Define all input params for derod - need to be sure most/all are the default from standard command line parser as we are not using that here
initparams["--rpc-bind"] = "127.0.0.1:20202"
initparams["--p2p-bind"] = "127.0.0.1:20201"
initparams["--getwork-bind"] = "127.0.0.1:20200"

chain := derodpkg.InitializeDerod(initparams)
```

## Starting DERO Daemon
After initializing, you can simply pass the returned chain (of type [*blockchain.Blockchain](https://github.com/deroproject/derohe/blob/main/blockchain/blockchain.go#L59)) as your input param and assign the output rpcserver of type [*rpc.RPCServer](https://github.com/deroproject/derohe/blob/main/cmd/derod/rpc/websocket_server.go#L54)

```go
rpcserver := derodpkg.StartDerod(chain)
```

## Exiting DERO Daemon
After the daemon has been initialized and started or is already running. You can pass through chain and rpcserver to the StopDerod() function to initiate a blockchain shutdown.

```go
derodpkg.StopDerod(rpcserver, chain)
```