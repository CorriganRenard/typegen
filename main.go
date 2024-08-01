package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/CorriganRenard/typegen/utils"
	"github.com/spf13/viper"
)

func main() {

	flag.Usage = func() {
		fmt.Printf("Usage: go run z_new_store.go ObjectName\n\n")
		fmt.Printf("ObjectName should be replaced with the name of an object in the system that corresponds to a database table. This script will produce a new sqlstore for the specified object.  The struct definition for ObjectName must already exist in order for the resulting code to compile.\n\n")
		fmt.Printf("ObjectName will be converted into object-name when used in file names and object_name for JSON and database field use.  object-name.go and object-name_test.go will be generated (and will not be overwritten if they already exiist).  The files created by this script are meant as a starting point and can and should be modified by the developer to accommodate the needs of this specific object in the system - the point of this script is to make it quick to set up a new type and generate some of the boilerplate code involved.\n\n")
	}

	configDirF := flag.String("config-dir", "./", "Load configuration from the specified directory")

	flag.Parse()

	viper.SetConfigName("typegen")

	configDirFile, _ := filepath.Abs(*configDirF)
	log.Printf("configDirF: %v configDirFile: %v", configDirF, configDirFile)

	viper.AddConfigPath(configDirFile)
	viper.SetConfigType("toml")

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		fmt.Printf("config not found in: %q\n", configDirF)
		os.Exit(12)
	}

	typesP := viper.GetString("types")
	handlersP := viper.GetString("web-handlers")
	sqlstoreP := viper.GetString("sqlstore")

	log.Printf("types package: %v handlers package: %v sqlstore package: %v", typesP, handlersP, sqlstoreP)

	baseDir, _ := filepath.Abs(viper.GetString("base-dir"))
	schemaFile := filepath.Join(baseDir, viper.GetString("schema"))

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, schemaFile, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	// parses the schema file and returns template struct
	savedWons := parseWons(file)

	for _, won := range savedWons {
		if len(won.StructName) == 0 {
			fmt.Printf("ObjectName cannot be empty\n")
			os.Exit(11)
		}
		if !(unicode.IsLetter(rune(won.StructName[0])) && unicode.IsUpper(rune(won.StructName[0]))) {
			fmt.Printf("%q does not start with an upper case letter\n", won.StructName)
			os.Exit(12)
		}

		oNameDash, _, _ := getVariations(won.StructName)

		for _, tmpl := range []string{"sqlstore2.tmpl", "sqlstore2_test.tmpl", "types.tmpl"} {
			testSuffix := ""
			if strings.Contains(tmpl, "_test") {
				testSuffix = "_test"
			}
			var packagePath string
			if strings.HasPrefix(tmpl, "types") {
				packagePath = typesP
			} else if strings.HasPrefix(tmpl, "sqlstore") {

				packagePath = sqlstoreP
			} else if strings.HasPrefix(tmpl, "handler") {
				packagePath = handlersP
			} else {
				log.Fatalf("malformed tmpl prefix: %v must be one of []{sqlstore, types, handler}", tmpl)
			}
			fpath := filepath.Join(baseDir, packagePath, oNameDash+testSuffix+".go")
			if _, err := os.Stat(fpath); err != nil {
				// file doesn't exist -- write it
				file, err := os.Create(fpath)
				if err != nil {
					log.Fatal(err)
				}
				defer file.Close()

				tmplFile := tmpl
				t, err := template.New(tmplFile).ParseFiles(filepath.Join("tmpl", tmplFile))
				if err != nil {
					log.Fatal(err)
				}
				err = t.Execute(file, won)
				if err != nil {
					log.Fatal(err)
				}

				file.Close()

				runGofmt(fpath)
			}
		}
	}
}

func parseWons(file ast.Node) []Won {

	var savedWons []Won
	var won Won

	ast.Inspect(file, func(x ast.Node) bool {
		log.Printf("inspecting")
		st, ok := x.(*ast.TypeSpec)
		if ok {
			log.Printf("struct name: %s", st.Name.Name)

			won = Won{StructName: st.Name.Name}

			won.NameDash, won.NameUnderscore, won.NameCamel = getVariations(won.StructName)

		}
		s, ok := x.(*ast.StructType)
		if !ok {
			return true
		}

		for _, field := range s.Fields.List {

			var isPK, isFK bool
			var saveField StructField
			saveField.FieldName = field.Names[0].Name
			saveField.FieldType = fmt.Sprintf("%s", field.Type)
			comment := field.Comment.Text()
			jsonFields := strings.Split(strings.TrimSpace(comment), ",")
			log.Printf("field comment: %v", comment)
			log.Printf("field split: %#v", jsonFields)

			var possibleJSONFields []StructField
			for _, v := range jsonFields {
				if v == "" {
					continue
				}
				newJSONField := StructField{}
				newJSONField.FieldName = v
				newJSONField.FieldType = "string"
				newJSONField.NameDash, newJSONField.NameUnderscore, newJSONField.NameCamel = getVariations(v)
				possibleJSONFields = append(possibleJSONFields, newJSONField)
			}
			log.Printf("possibleJsonFields: %#v", possibleJSONFields)

			saveField.NameDash, saveField.NameUnderscore, saveField.NameCamel = getVariations(saveField.FieldName)

			// fmt.Printf("Field: %s\n", field.Names[0].Name)
			// fmt.Printf("type: %s\n", field.Type)

			if field.Tag != nil {
				fmt.Printf("Tag:   %s\n", field.Tag.Value)
				tagSplit := strings.Split(field.Tag.Value, ":")
				saveField.TagType = strings.Trim(tagSplit[0], "\"`")

				tagValueSplit := strings.Split(tagSplit[1], ",")
				saveField.TagValue = strings.Trim(tagValueSplit[0], "\"`")
				if len(tagValueSplit) > 1 {
					saveField.TagValue2 = strings.Trim(tagValueSplit[1], "\"`")
				}
				log.Printf("tagtype: %v", saveField.TagType)
				switch saveField.TagType {
				case "rel": // insert pk, fk
					switch saveField.TagValue {
					case "primary_key":
						isPK = true
					case "foreign_key":
						isFK = true
					}
				case "type": // insert enum, json or time fields
					switch saveField.TagValue {
					case "enum": // get enum type, insert enumfields
						// enumType := saveField.TagValue2
						saveField.Enums = jsonFields
					case "json_struct": // get struct fields from comment, insert jsonfields
						saveField.JSONFields = possibleJSONFields
					case "time":
					}
				default:
				}

			}
			if isPK && len(won.PrimaryKeyField.FieldName) == 0 {
				won.PrimaryKeyField = saveField
			}
			if isFK && len(won.ForeignKeyField.FieldName) == 0 {
				won.ForeignKeyField = saveField
			}

			//log.Printf("json fields: %v", won.JSONFields)

			won.StructFields = append(won.StructFields, saveField)
		}
		savedWons = append(savedWons, won)
		return false
	})

	return savedWons
}

func getVariations(oName string) (dash, underscore, camel string) {
	oNameParts, err := utils.SplitObjWords(oName)
	if err != nil {
		log.Printf("unabled to splitObjWords for %v:  %v", oName, err)
	}
	return strings.Join(oNameParts, "-"), strings.Join(oNameParts, "_"), lowerCamelJoin(oNameParts)

}

type Won struct {
	StructName     string
	NameDash       string
	NameUnderscore string
	NameCamel      string

	StructFields    []StructField
	PrimaryKeyField StructField
	ForeignKeyField StructField
	EnumFields      []StructField
	JSONFields      []StructField
	TimeFields      []StructField
}
type StructField struct {
	FieldName      string
	NameDash       string
	NameUnderscore string
	NameCamel      string

	FieldType string
	TagType   string
	TagValue  string
	TagValue2 string
	Enums     []string

	JSONFields []StructField
}

func runGofmt(fp string) error {

	// use goimports instead if available
	gofmtPath, err := exec.LookPath("goimports")
	if err != nil {
		gofmtPath = "gofmt"
	}

	args := []string{"-w", fp}
	b, err := exec.Command(gofmtPath, args...).CombinedOutput()
	log.Printf("Command: %s %v; output:\n%s", gofmtPath, args, b)
	return err
}

// ["some","words","here"] -> "someWordsHere"
func lowerCamelJoin(in []string) string {
	var buf bytes.Buffer
	for i, el := range in {
		if i == 0 {
			buf.WriteString(el)
			continue
		}
		if len(el) == 0 {
			continue
		}
		buf.WriteRune(unicode.ToUpper(rune(el[0])))
		buf.WriteString(el[1:])
	}
	return buf.String()
}

/*
	t.Run("Select", func(t *testing.T) {
		assert := assert.New(t)
		user := User{
			Username: "select-test1@example.com",
			Email:    "select-test1@example.com",
		}
		assert.NoError(sqlStore.User().Insert(ctx, &user))
		assert.NotEmpty(user.UserID)
		user = User{
			Username: "select-test2@example.com",
			Email:    "select-test2@example.com",
		}
		assert.NoError(sqlStore.User().Insert(ctx, &user))
		userList, err := sqlStore.User().Select(ctx,
			tmetautil.Criteria{tmetautil.Criterion{Field: "username", Op: tmetautil.LikeOp, Value: "select-test%"}},
			tmetautil.OrderByList{tmetautil.OrderBy{Field: "username"}},
			10, 0, nil)
		assert.NoError(err)
		assert.Len(userList, 2)
	})

	t.Run("SelectUsernameLike", func(t *testing.T) {
		assert := assert.New(t)
		user := User{
			Username: "select-username-like-test1@example.com",
			Email:    "select-username-like-test1@example.com",
		}
		assert.NoError(sqlStore.User().Insert(ctx, &user))
		assert.NotEmpty(user.UserID)
		user = User{
			Username: "select-username-like-test2@example.com",
			Email:    "select-username-like-test2@example.com",
		}
		assert.NoError(sqlStore.User().Insert(ctx, &user))
		// use the streamed approach
		var c int
		_, err := sqlStore.User().Select(ctx,
			tmetautil.Criteria{tmetautil.Criterion{Field: "username", Op: tmetautil.LikeOp, Value: "select-username-like-test%"}},
			tmetautil.OrderByList{tmetautil.OrderBy{Field: "username"}},
			10, 0, func(o User) error {
				c++
				return nil
			})
		assert.NoError(err)
		assert.Equal(2, c)
	})
*/
