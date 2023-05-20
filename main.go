package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/samber/lo"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/descriptorpb"
)

var (
	pTemplate = pflag.StringP("template", "T", "", "template proto file")
	pInclude  = pflag.StringP("include", "I", "", "include proto file")
	pOutput   = pflag.StringP("output", "O", "", "output proto file")
	pPackage  = pflag.StringP("package", "P", "", "package name")

	allMessageDescriptors = map[string]*descriptorpb.DescriptorProto{}
)

func main() {
	pflag.Parse()
	if *pTemplate == "" || *pOutput == "" {
		fmt.Println("usage: oneproto -t template.proto -O output.proto -p example.com -I path/to/protos 1.proto 2.proto")
		os.Exit(1)
	}

	var entries []string
	for _, input := range pflag.Args() {
		if err := fs.WalkDir(os.DirFS(input), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".proto") {
				return err
			}
			entries = append(entries, path)
			return nil
		}); err != nil {
			log.Fatalln(err)
		}
	}

	templateContent, err := os.ReadFile(*pTemplate)
	if err != nil {
		fmt.Printf("unable to read template file: %v\n", err)
		os.Exit(1)
	}

	parser := protoparse.Parser{}
	files, err := parser.ParseFilesButDoNotLink(lo.Map(entries, func(entry string, index int) string {
		return filepath.Join(*pInclude, entry)
	})...)
	if err != nil {
		log.Fatalln(err)
	}

	buf := &Buffer{buf: &bytes.Buffer{}}
	buf.Printf(string(templateContent))

	// group files by package name
	packageFileGroups := make(map[string][]*descriptorpb.FileDescriptorProto)
	for _, file := range files {
		packageFileGroups[file.GetPackage()] = append(packageFileGroups[file.GetPackage()], file)
		for _, message := range file.GetMessageType() {
			allMessageDescriptors[trimPackageFromName(fmt.Sprintf("%s.%s", file.GetPackage(), message.GetName()))] = message
		}
	}

	for pkg, packageFiles := range packageFileGroups {
		var paths []string
		if pkg != *pPackage {
			paths = strings.Split(trimPackageFromName(pkg), ".")
			for i, path := range paths {
				buf.Printf("%smessage %s {", strings.Repeat(" ", i*4), path)
			}
		}
		indentLevel := len(paths)

		for _, file := range packageFiles {
			for _, enum := range file.EnumType {
				generateEnum(buf, indentLevel, enum)
				buf.Printf("")
			}

			for _, service := range file.Service {
				generateService(buf, indentLevel, service)
				buf.Printf("")
			}

			for _, message := range file.GetMessageType() {
				generateMessage(buf, indentLevel, message)
				buf.Printf("")
			}
		}

		for i := range paths {
			buf.Printf("%s}", strings.Repeat(" ", i*4))
		}
		buf.Printf("")
	}
	if err := os.WriteFile(*pOutput, buf.Bytes(), 0644); err != nil {
		log.Fatalln(err)
	}
}

type Buffer struct {
	buf *bytes.Buffer
}

func (b *Buffer) Printf(f string, i ...any) {
	_, _ = b.buf.WriteString(fmt.Sprintf(f+"\n", i...))
}

func (b *Buffer) String() string {
	return b.buf.String()
}

func (b *Buffer) Bytes() []byte {
	return b.buf.Bytes()
}

func stringifyUninterpretedOption(opt *descriptorpb.UninterpretedOption) string {
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

func stringifyField(message *descriptorpb.DescriptorProto, field *descriptorpb.FieldDescriptorProto) string {
	repeated := field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	if field.Type == nil {
		name := field.GetTypeName()
		for _, nested := range message.NestedType {
			if name == nested.GetName() && strings.HasSuffix(name, "Entry") { // map entry
				return fmt.Sprintf("map<%s,%s>", stringifyField(nested, nested.Field[0]), stringifyField(nested, nested.Field[1]))
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

func generateService(buf *Buffer, indentLevel int, service *descriptorpb.ServiceDescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%sservice %s {", indent, service.GetName())
	generateHeadOptions(buf, indent, service.GetOptions().GetUninterpretedOption())
	for _, method := range service.Method {
		buf.Printf("%s    rpc %s(%s) returns (%s) {", indent, method.GetName(), method.GetInputType(), method.GetOutputType())
		for _, opt := range method.GetOptions().GetUninterpretedOption() {
			buf.Printf("%s        option %s;", indent, stringifyUninterpretedOption(opt))
		}
		buf.Printf("%s    }", indent)
		buf.Printf("")
	}
	buf.Printf("%s}", indent)
}

func generateEnum(buf *Buffer, indentLevel int, enum *descriptorpb.EnumDescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%senum %s {", indent, enum.GetName())
	generateHeadOptions(buf, indent, enum.GetOptions().GetUninterpretedOption())

	for _, value := range enum.Value {
		buf.Printf("%s    %s  = %d%s;", indent, value.GetName(), value.GetNumber(), stringifyValueOptions(value.GetOptions().GetUninterpretedOption()))
	}
	buf.Printf("%s}", indent)
}

func generateMessage(buf *Buffer, indentLevel int, message *descriptorpb.DescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%smessage %s {", indent, message.GetName())
	generateHeadOptions(buf, indent, message.GetOptions().GetUninterpretedOption())
	for _, opt := range message.GetOptions().GetUninterpretedOption() {
		if isOptionOneProtoExtends(opt) {
			name := trimPackageFromName(string(opt.GetStringValue()))
			buf.Printf("%s    // ↓↓↓↓↓ extends %s", indent, name)
			parent := allMessageDescriptors[name]
			for _, field := range parent.GetField() {
				buf.Printf("%s    %s %s = %d%s;", indent, stringifyField(message, field), field.GetName(), field.GetNumber(), stringifyValueOptions(field.GetOptions().GetUninterpretedOption()))
			}
			buf.Printf("%s    // ↑↑↑↑↑ extends %s", indent, name)
			buf.Printf("")
		}
	}
	for _, field := range message.GetField() {
		buf.Printf("%s    %s %s = %d%s;", indent, stringifyField(message, field), field.GetName(), field.GetNumber(), stringifyValueOptions(field.GetOptions().GetUninterpretedOption()))
	}

	for _, enum := range message.EnumType {
		buf.Printf("")
		generateEnum(buf, indentLevel+1, enum)
	}

	for _, nested := range message.NestedType {
		if nested.GetOptions().GetMapEntry() {
			continue
		}
		buf.Printf("")
		generateMessage(buf, indentLevel+1, nested)
	}
	buf.Printf("%s}", indent)
}

func generateHeadOptions(buf *Buffer, indent string, options []*descriptorpb.UninterpretedOption) {
	if len(options) == 0 {
		return
	}
	for _, opt := range options {
		if isOptionOneProtoExtends(opt) {
			continue
		}
		buf.Printf("%s    option %s;", indent, stringifyUninterpretedOption(opt))
	}
	buf.Printf("")
}

func stringifyValueOptions(options []*descriptorpb.UninterpretedOption) string {
	if len(options) == 0 {
		return ""
	}
	var opts []string
	for _, opt := range options {
		opts = append(opts, stringifyUninterpretedOption(opt))
	}
	return fmt.Sprintf(" [%s]", strings.Join(opts, ", "))
}

func isOptionOneProtoExtends(option *descriptorpb.UninterpretedOption) bool {
	if names := option.GetName(); len(names) > 0 && names[0].GetNamePart() == "oneproto.extends" {
		return true
	}
	return false
}

func trimPackageFromName(name string) string {
	return strings.TrimPrefix(name, *pPackage+".")
}
