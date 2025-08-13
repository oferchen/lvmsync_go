# Flow

## Before
```
main -> Configure -> selectTransport -> SetupGRPC -> ExecuteClient -> SyncLogger
NewSSHClient -> setupHostKeyCallback (verify ignored) -> knownhosts.New -> dialWithRetry
```

## After
```
main -> Configure -> SetupGRPC -> ExecuteClient -> SyncLogger
NewSSHClient -> setupHostKeyCallback (honors verify) -> [verify true: knownhosts.New, verify false: ssh.InsecureIgnoreHostKey] -> dialWithRetry
```

`selectTransport` previously depended on an unused transport package. The new flow removes the unused dependency and warns when a transport is requested.
