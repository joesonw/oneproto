package oneprotou_til

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/descriptorpb"
)

func StringifyUninterpretedOption(opt *descriptorpb.UninterpretedOption) string {
	var value string
	if v := opt.IdentifierValue; v != nil {
		value = *v
	} else if v := opt.DoubleValue; v != nil {
		value = strconv.FormatFloat(*v, 'f', -1, 64)
	} else if v := opt.AggregateValue; v != nil {
		value = fmt.Sprintf("{%s}", *v)
	} else if v := opt.StringValue; v != nil {
		value = fmt.Sprintf("'%s'", v)
	} else if v := opt.PositiveIntValue; v != nil {
		value = strconv.Itoa(int(*v))
	} else if v := opt.NegativeIntValue; v != nil {
		value = strconv.Itoa(int(*v))
	}
	return fmt.Sprintf("(%s) = %s", strings.Join(lo.Map(opt.GetName(), func(name *descriptorpb.UninterpretedOption_NamePart, index int) string {
		return name.GetNamePart()
	}), ","), value)
}

func StringifyField(message *descriptorpb.DescriptorProto, field *descriptorpb.FieldDescriptorProto) string {
	repeated := field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	if field.Type == nil {
		name := field.GetTypeName()
		for _, nested := range message.GetNestedType() {
			if name == nested.GetName() && strings.HasSuffix(name, "Entry") { // map entry
				return fmt.Sprintf("map<%s,%s>", StringifyField(nested, nested.Field[0]), StringifyField(nested, nested.Field[1]))
			}
		}
		if repeated {
			return "repeated " + name
		}
		return name
	}
	name := strings.ToLower(field.GetType().String()[5:])
	if repeated {
		name = "repeated " + name
	}
	return name
}

func StringifyValueOptions(options []*descriptorpb.UninterpretedOption) string {
	if len(options) == 0 {
		return ""
	}
	var opts []string
	for _, opt := range options {
		opts = append(opts, StringifyUninterpretedOption(opt))
	}
	return fmt.Sprintf(" [%s]", strings.Join(opts, ", "))
}
