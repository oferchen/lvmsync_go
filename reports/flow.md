# Flow

## Before
```
main -> Configure -> selectTransport -> SetupGRPC -> ExecuteClient -> SyncLogger
NewSSHClient -> setupHostKeyCallback (verify ignored) -> knownhosts.New -> dialWithRetry
```

## After
```
main -> Configure -> selectTransport -> SetupGRPC -> ExecuteClient -> SyncLogger
NewSSHClient -> [key provided: loadPrivateKey | none: sshAgentAuth (context timeout)] -> setupHostKeyCallback (honors verify) -> [verify true: knownhosts.New, verify false: ssh.InsecureIgnoreHostKey] -> dialWithRetry
```

`selectTransport` now fails fast when a transport is specified, preventing silent misconfiguration.

## Serve Shutdown
```
serve.Run -> context.WithCancel -> startServer -> [error or exit] -> cancel -> logger.Sync
```

Wrapping the server with a cancellable context and syncing the logger on shutdown ensures resources are released and logs are flushed.
