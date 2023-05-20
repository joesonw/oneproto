# How to use

```
go install github.com/joesonw/oneproto
oneproto -I ./proto -O output.proto -T template.proto -P example.com ./proto 
```

for more details, please check [example](./example)


## Extend another message 

One proto now support extend another message (merge) by specifying `extend` in the message options.

```proto

Please see [user.proto](./example/proto/user.proto) for example

```proto
// import oneproto.proto. This will not effect the tool nor output, this is to make the IDE happy.
// You might want to include "go list --json github.com/joesonw/oneproto | jq -r '.Dir'" 

import "oneproto.proto";
```
