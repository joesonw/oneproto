package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/samber/lo"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/descriptorpb"

	oneprotou_til "github.com/joesonw/oneproto/util"
)

var (
	pTemplate = pflag.StringP("template", "T", "", "template proto file")
	pInclude  = pflag.StringP("include", "I", "", "include proto file")
	pOutput   = pflag.StringP("output", "O", "", "output proto file")
	pPackage  = pflag.StringP("package", "P", "", "package name")
	pOptions  = pflag.Bool("options", false, "extend parent options")

	allMessageDescriptors = map[string]*descriptorpb.DescriptorProto{}
	parentResolvedMap     = map[*descriptorpb.DescriptorProto]bool{}
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

	parser := protoparse.Parser{
		IncludeSourceCodeInfo: true,
	}
	files, err := parser.ParseFilesButDoNotLink(lo.Map(entries, func(entry string, index int) string {
		return filepath.Join(*pInclude, entry)
	})...)
	if err != nil {
		log.Fatalln(err)
	}

	buf := oneprotou_til.NewBuffer()
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
				oneprotou_til.GenerateEnum(buf, indentLevel, enum)
				buf.Printf("")
			}

			for _, service := range file.Service {
				oneprotou_til.GenerateService(buf, indentLevel, service)
				buf.Printf("")
			}

			for _, message := range file.GetMessageType() {
				resolveMessageExtends(buf, indentLevel, message)
				oneprotou_til.GenerateMessage(buf, indentLevel, message)
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

func resolveMessageExtends(buf *oneprotou_til.Buffer, indentLevel int, message *descriptorpb.DescriptorProto) {
	if parentResolvedMap[message] {
		return
	}
	parentResolvedMap[message] = true
	for i, option := range message.GetOptions().GetUninterpretedOption() {
		if isOptionOneProtoExtends(option) {
			message.Options.UninterpretedOption = append(message.Options.UninterpretedOption[:i], message.Options.UninterpretedOption[i+1:]...)
			buf.Printf("%s// extends %s", strings.Repeat(" ", 4*indentLevel), option.GetStringValue())
			parent := allMessageDescriptors[trimPackageFromName(string(option.GetStringValue()))]
			if parent == nil {
				log.Fatalf("unable to find message %s", option.GetStringValue())
			}
			resolveMessageExtends(buf, indentLevel, parent)
			message.Field = append(message.Field, parent.Field...)
			if *pOptions {
				message.Options.UninterpretedOption = append(message.Options.UninterpretedOption, parent.Options.UninterpretedOption...)
			}
		}
	}
	sort.Slice(message.Field, func(i, j int) bool {
		return message.Field[i].GetNumber() < message.Field[j].GetNumber()
	})
}

func trimPackageFromName(name string) string {
	return strings.TrimPrefix(name, *pPackage+".")
}

func isOptionOneProtoExtends(option *descriptorpb.UninterpretedOption) bool {
	if names := option.GetName(); len(names) > 0 && names[0].GetNamePart() == "oneproto.extends" {
		return true
	}
	return false
}
