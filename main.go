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

	oneproto_util "github.com/joesonw/oneproto/util"
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

type PackageFileGroup struct {
	name        string
	fullName    string
	files       []*descriptorpb.FileDescriptorProto
	subPackages map[string]*PackageFileGroup
}

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

	buf := oneproto_util.NewBuffer()
	buf.Printf(string(templateContent))

	// group files by package name
	packageFileGroup := &PackageFileGroup{
		subPackages: map[string]*PackageFileGroup{},
	}
	for _, file := range files {
		pkg := file.GetPackage()
		var paths []string
		if pkg != *pPackage {
			paths = strings.Split(trimPackageFromName(pkg), ".")
		}

		if len(paths) == 0 {
			packageFileGroup.files = append(packageFileGroup.files, file)
		} else {
			var currentGroup = packageFileGroup
			for i, path := range paths {
				if currentGroup.subPackages[path] == nil {
					currentGroup.subPackages[path] = &PackageFileGroup{
						name:        path,
						fullName:    strings.Join(paths[:i+1], "."),
						subPackages: map[string]*PackageFileGroup{},
					}
				}
				currentGroup = currentGroup.subPackages[path]
			}
			currentGroup.files = append(currentGroup.files, file)
		}

		for _, message := range file.GetMessageType() {
			fillAllMessageDescriptors(file.GetPackage(), message)
		}
	}

	var iteratePackageGroup func(group *PackageFileGroup, identLevel int)
	iteratePackageGroup = func(group *PackageFileGroup, identLevel int) {
		for _, file := range group.files {
			for _, service := range file.Service {
				oneproto_util.GenerateService(buf, 0, service)
				buf.Printf("")
			}
		}

		if group.name != "" {
			buf.Printf("%smessage %s {", strings.Repeat(" ", identLevel*4), group.name)
		}

		for _, file := range group.files {
			for _, enum := range file.EnumType {
				oneproto_util.GenerateEnum(buf, identLevel+1, enum)
				buf.Printf("")
			}

			for _, message := range file.GetMessageType() {
				resolveMessageExtends(message)
				oneproto_util.GenerateMessage(buf, identLevel+1, message)
				buf.Printf("")
			}

			oneproto_util.GenerateExtensions(buf, identLevel+1, nil, file.GetExtension())
			buf.Printf("")
		}

		for _, subPackage := range group.subPackages {
			iteratePackageGroup(subPackage, identLevel+1)
		}
		if group.name != "" {
			buf.Printf("%s}", strings.Repeat(" ", identLevel*4))
		}
		buf.Printf("")
	}
	iteratePackageGroup(packageFileGroup, -1)

	if err := os.WriteFile(*pOutput, buf.Bytes(), 0644); err != nil {
		log.Fatalln(err)
	}
}

func resolveMessageExtends(message *descriptorpb.DescriptorProto) {
	if parentResolvedMap[message] {
		return
	}
	for _, nested := range message.GetNestedType() {
		resolveMessageExtends(nested)
	}

	parentResolvedMap[message] = true
	var options []*descriptorpb.UninterpretedOption
	var keepOptions []*descriptorpb.UninterpretedOption
	var fields []*descriptorpb.FieldDescriptorProto
	for _, option := range message.GetOptions().GetUninterpretedOption() {
		if isOptionOneProtoExtends(option) {
			parent := allMessageDescriptors[trimPackageFromName(string(option.GetStringValue()))]
			if parent == nil {
				log.Fatalf("unable to find message %s", option.GetStringValue())
			}
			resolveMessageExtends(parent)
			fields = append(fields, parent.Field...)
			options = append(options, parent.GetOptions().GetUninterpretedOption()...)
		} else {
			keepOptions = append(keepOptions, option)
		}
	}
	if message.Options != nil {
		message.Options.UninterpretedOption = keepOptions
	}

	message.Field = append(message.Field, fields...)
	if *pOptions {
		if message.Options == nil {
			message.Options = &descriptorpb.MessageOptions{}
		}
		for i := range options {
			if isOptionOneProtoExtends(options[i]) {
				options = append(options[:i], options[i+1:]...)
			}
		}
	}
	message.Options.UninterpretedOption = append(message.Options.UninterpretedOption, options...)
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

func fillAllMessageDescriptors(pkg string, message *descriptorpb.DescriptorProto) {
	allMessageDescriptors[trimPackageFromName(fmt.Sprintf("%s.%s", pkg, message.GetName()))] = message
	for _, sub := range message.GetNestedType() {
		fillAllMessageDescriptors(fmt.Sprintf("%s.%s", pkg, message.GetName()), sub)
	}
}
