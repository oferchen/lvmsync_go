# Flow

## Before
```
main -> Configure -> selectTransport -> SetupGRPC -> ExecuteClient -> SyncLogger
```

## After
```
main -> Configure -> SetupGRPC -> ExecuteClient -> SyncLogger
```

`selectTransport` previously depended on an unused transport package. The new flow removes the unused dependency and warns when a transport is requested.
