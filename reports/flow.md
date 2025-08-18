# Flow

## Before
```
NewSSHClient -> setupHostKeyCallback (verify ignored) -> knownhosts.New -> dialWithRetry
```

## After
```
NewSSHClient -> [key provided: loadPrivateKey | none: sshAgentAuth (context timeout)] -> setupHostKeyCallback (honors verify) -> [verify true: knownhosts.New, verify false: ssh.InsecureIgnoreHostKey] -> dialWithRetry
```

`selectTransport` now fails fast when a transport is specified, preventing silent misconfiguration.
