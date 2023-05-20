package oneprotou_til

import (
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
)

func GenerateService(buf *Buffer, indentLevel int, service *descriptorpb.ServiceDescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%sservice %s {", indent, service.GetName())
	GenerateHeadOptions(buf, indent, service.GetOptions().GetUninterpretedOption())
	for _, method := range service.Method {
		buf.Printf("%s    rpc %s(%s) returns (%s) {", indent, method.GetName(), method.GetInputType(), method.GetOutputType())
		for _, opt := range method.GetOptions().GetUninterpretedOption() {
			buf.Printf("%s        option %s;", indent, StringifyUninterpretedOption(opt))
		}
		buf.Printf("%s    }", indent)
		buf.Printf("")
	}
	buf.Printf("%s}", indent)
}

func GenerateEnum(buf *Buffer, indentLevel int, enum *descriptorpb.EnumDescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%senum %s {", indent, enum.GetName())
	GenerateHeadOptions(buf, indent, enum.GetOptions().GetUninterpretedOption())

	for _, value := range enum.Value {
		buf.Printf("%s    %s  = %d%s;", indent, value.GetName(), value.GetNumber(), StringifyValueOptions(value.GetOptions().GetUninterpretedOption()))
	}
	buf.Printf("%s}", indent)
}

func GenerateMessage(buf *Buffer, indentLevel int, message *descriptorpb.DescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%smessage %s {", indent, message.GetName())
	GenerateHeadOptions(buf, indent, message.GetOptions().GetUninterpretedOption())
	for _, field := range message.GetField() {
		buf.Printf("%s    %s %s = %d%s;", indent, StringifyField(message, field), field.GetName(), field.GetNumber(), StringifyValueOptions(field.GetOptions().GetUninterpretedOption()))
	}

	for _, enum := range message.EnumType {
		buf.Printf("")
		GenerateEnum(buf, indentLevel+1, enum)
	}

	for _, nested := range message.NestedType {
		if nested.GetOptions().GetMapEntry() {
			continue
		}
		buf.Printf("")
		GenerateMessage(buf, indentLevel+1, nested)
	}
	buf.Printf("%s}", indent)
}

func GenerateHeadOptions(buf *Buffer, indent string, options []*descriptorpb.UninterpretedOption) {
	if len(options) == 0 {
		return
	}
	for _, opt := range options {
		buf.Printf("%s    option %s;", indent, StringifyUninterpretedOption(opt))
	}
	buf.Printf("")
}
