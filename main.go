package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

// Copied from https://github.com/grpc-ecosystem/grpc-gateway/blob/master/protoc-gen-grpc-gateway/main.go
// BSD-3 licensed
func parseReq(r io.Reader) (*plugin.CodeGeneratorRequest, error) {
	log.Println("Parsing code generator request")
	input, err := ioutil.ReadAll(r)
	if err != nil {
		log.Printf("Failed to read code generator request: %v", err)
		return nil, err
	}
	req := new(plugin.CodeGeneratorRequest)
	if err = proto.Unmarshal(input, req); err != nil {
		log.Printf("Failed to unmarshal code generator request: %v", err)
		return nil, err
	}
	log.Println("Parsed code generator request")
	return req, nil
}

func parseParameters(req *plugin.CodeGeneratorRequest) {
	if req.Parameter != nil {
		for _, p := range strings.Split(req.GetParameter(), ",") {
			spec := strings.SplitN(p, "=", 2)
			if len(spec) == 1 {
				if err := flag.CommandLine.Set(spec[0], ""); err != nil {
					log.Fatalf("Cannot set flag %s", p)
				}
				continue
			}
			name, value := spec[0], spec[1]
			if err := flag.CommandLine.Set(name, value); err != nil {
				log.Fatalf("Cannot set flag %s", p)
			}
		}
	}
}

func fileFromReq(req *plugin.CodeGeneratorRequest, name string) (*descriptor.FileDescriptorProto, error) {
	for _, f := range req.ProtoFile {
		if f.GetName() == name {
			return f, nil
		}
	}
	return nil, fmt.Errorf("couldn't find file to generate: %s", name)
}

type Method struct {
	Name   string
	Input  string
	Output string
}

type Message struct {
	Name   string
	Fields []*Field
}

type Field struct {
	Comment string
	Name    string
	Type    string
}

type Vars struct {
	Name     string
	Package  string
	Methods  []*Method
	Messages []*Message
}

var (
	templatePath = flag.String("template_path", "", "path to template file")
	fileExt      = flag.String("file_ext", "", "file extension for new files")
)

func main() {
	// I think we just need this to get the flag internals set up
	flag.Parse()

	// Parse the input
	req, err := parseReq(os.Stdin)
	if err != nil {
		log.Fatalf("unable to parse protobuf: %v", err)
	}

	// Parse the parameters into flags
	parseParameters(req)

	// Load the template
	tmpl, err := template.ParseFiles(*templatePath)
	if err != nil {
		log.Fatalf("unable to parse template: %v", err)
	}

	var files []*plugin.CodeGeneratorResponse_File

	log.Printf("FileToGenerate %v", req.GetFileToGenerate())
	// TODO(termie): support multiple files

	for _, name := range req.GetFileToGenerate() {
		f, err := fileFromReq(req, name)
		if err != nil {
			log.Fatalf("couldn't find file: %v", err)
		}
		code := bytes.NewBuffer(nil)

		messages = []*Message{}
		for _, msg := range f.MessageType {
			m := &Message{
				Name:   *msg.Name,
				Fields: []*Field{},
			}

			for _, field := range msg.Field {
				m.Fields = append(m.Fields, &Field{
					Name: *field.Name,
					Type: getFieldType(field, *f.Package),
				})
			}

			messages = append(messages, m)
		}

		services := []*Service{}
		for _, svc := range f.Service {
			methods := []*Method{}
			for _, mth := range svc.Method {
				method := &Method{
					Name:   mth.GetName(),
					Input:  mth.GetInputType(),
					Output: mth.GetOutputType(),
				}
				methods = append(methods, method)
			}
			service := &Service{
				Name:    svc.GetName(),
				Methods: methods,
			}
		}

		tmpl.Execute(code, f)

		name := f.GetName()
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		output := fmt.Sprintf("%s.%s", base, *fileExt)

		files = append(files, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(output),
			Content: proto.String(strings.TrimLeft(code.String(), "\n")),
		})
	}

	emitFiles(files)
}

func emitFiles(out []*plugin.CodeGeneratorResponse_File) {
	emitResp(&plugin.CodeGeneratorResponse{File: out})
}

func emitError(err error) {
	emitResp(&plugin.CodeGeneratorResponse{Error: proto.String(err.Error())})
}

func emitResp(resp *plugin.CodeGeneratorResponse) {
	buf, err := proto.Marshal(resp)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(buf); err != nil {
		log.Fatal(err)
	}
}

func getFieldType(field *descriptor.FieldDescriptorProto, pkg string) string {
	ret := "any" // unknonwn

	switch *field.Type {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE,
		descriptor.FieldDescriptorProto_TYPE_FLOAT,
		descriptor.FieldDescriptorProto_TYPE_INT32,
		descriptor.FieldDescriptorProto_TYPE_FIXED32,
		descriptor.FieldDescriptorProto_TYPE_UINT32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED32,
		descriptor.FieldDescriptorProto_TYPE_SINT32:
	case descriptor.FieldDescriptorProto_TYPE_INT64,
		descriptor.FieldDescriptorProto_TYPE_UINT64,
		descriptor.FieldDescriptorProto_TYPE_FIXED64,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_SINT64:
		// javascript doesn't support 64bit ints
		ret = "string"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		ret = "boolean"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		ret = "string"
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		ret = strings.TrimPrefix(*field.TypeName, fmt.Sprintf(".%s.", pkg))
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		ret = "UNKNONWN TYPE"
	}

	if *field.Label == descriptor.FieldDescriptorProto_LABEL_REPEATED {
		ret += "[]"
	}

	return ret
}
