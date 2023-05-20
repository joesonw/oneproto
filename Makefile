.PHONY: all
all:
	protoc --go_out=./ --go_opt=module=github.com/joesonw/oneproto oneproto.proto