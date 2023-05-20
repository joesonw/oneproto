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

	buf := oneprotou_til.NewBuffer()
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
			allMessageDescriptors[trimPackageFromName(fmt.Sprintf("%s.%s", file.GetPackage(), message.GetName()))] = message
		}
	}

	var iteratePackageGroup func(group *PackageFileGroup, identLevel int)
	iteratePackageGroup = func(group *PackageFileGroup, identLevel int) {
		if group.name != "" {
			buf.Printf("%smessage %s {", strings.Repeat(" ", identLevel*4), group.name)
		}

		for _, file := range group.files {
			for _, enum := range file.EnumType {
				oneprotou_til.GenerateEnum(buf, identLevel+1, enum)
				buf.Printf("")
			}

			for _, service := range file.Service {
				oneprotou_til.GenerateService(buf, identLevel+1, service)
				buf.Printf("")
			}

			for _, message := range file.GetMessageType() {
				resolveMessageExtends(buf, identLevel+1, message)
				oneprotou_til.GenerateMessage(buf, identLevel+1, message)
				buf.Printf("")
			}
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
			if message.Options == nil {
				message.Options = &descriptorpb.MessageOptions{}
			}
			if *pOptions {
				message.Options.UninterpretedOption = append(message.Options.UninterpretedOption, parent.GetOptions().GetUninterpretedOption()...)
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
