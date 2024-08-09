package main

import (
	"bytes"
	"embed"
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

	_ "embed"

	"github.com/CorriganRenard/typegen/utils"
	"github.com/spf13/viper"
)

func main() {

	flag.Usage = func() {
		fmt.Printf("Usage: typegen \n\n")
		fmt.Printf("reads from default config file 'typgen.toml' in current directory.\n\n")
		fmt.Printf("config file should contian schema.go filename, directory names for generated files, see example.\n\n")
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
		fmt.Printf("config not found in: %q\n", *configDirF)
		os.Exit(12)
	}

	//go:embed tmpl
	var content embed.FS
	parseFS := false

	dir, err := os.ReadDir("tmpl")
	if err != nil {
		parseFS = true
		dir, err = content.ReadDir(".")
		if err != nil {
			log.Fatal(err)
		}
	}

	var tmplMap = make(map[string]string)
	for _, v := range dir {
		prefix := strings.TrimSuffix(v.Name(), ".tmpl")
		prefix = strings.TrimSuffix(prefix, "_test")
		// log.Printf("tmpl %v dir %v", v.Name(), viper.GetString(prefix))
		tmplMap[v.Name()] = viper.GetString(prefix)
	}

	log.Printf("%#v", tmplMap)

	baseDir, _ := filepath.Abs(viper.GetString("base-dir"))
	schemaFile := filepath.Join(baseDir, viper.GetString("schema"))

	basePackage := viper.GetString("base-package")

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, schemaFile, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	// parses the schema file and returns template struct
	savedWons := parseWons(file)

	log.Printf("len wons: %v", len(savedWons))
	for _, won := range savedWons {
		won.BasePackage = basePackage
		if len(won.StructName) == 0 {
			fmt.Printf("ObjectName cannot be empty %q\n", won.StructName)
			os.Exit(11)
		}
		if !(unicode.IsLetter(rune(won.StructName[0])) && unicode.IsUpper(rune(won.StructName[0]))) {
			fmt.Printf("%q does not start with an upper case letter\n", won.StructName)
			os.Exit(12)
		}

		oNameDash, _, _, _ := getVariations(won.StructName)

		for tmpl, packagePath := range tmplMap {
			fileExt := filepath.Ext(packagePath)
			packagePath = strings.TrimSuffix(packagePath, fileExt)
			testSuffix := ""
			if strings.Contains(tmpl, "_test") {
				testSuffix = "_test"
			}
			fpath := filepath.Join(baseDir, packagePath, oNameDash+testSuffix+fileExt)
			if _, err := os.Stat(fpath); err != nil {
				// file doesn't exist -- write it

				if _, err := os.Stat(filepath.Join(baseDir, packagePath)); os.IsNotExist(err) {
					log.Printf("creating dir: %v", filepath.Join(baseDir, packagePath))
					err = os.MkdirAll(filepath.Join(baseDir, packagePath), 0700) // Create the dir
					if err != nil {
						log.Fatal(err)
					}
				}

				file, err := os.Create(fpath)
				if err != nil {
					log.Fatal(err)
				}
				defer file.Close()
				var t *template.Template
				if parseFS {
					t, err = template.ParseFS(content, tmpl)
					if err != nil {
						log.Fatal(err)
					}

				} else {
					t, err = template.ParseFiles(filepath.Join("tmpl", tmpl))
					if err != nil {
						log.Fatal(err)
					}
				}

				err = t.Execute(file, won)
				if err != nil {
					log.Fatal(err)
				}

				file.Close()

				if fileExt == ".go" {
					runGofmt(fpath)

				}
			}
		}
	}
}

func parseWons(file ast.Node) []Won {

	var savedWons []Won
	var won = NewWon()

	ast.Inspect(file, func(x ast.Node) bool {
		log.Printf("inspecting")
		st, ok := x.(*ast.TypeSpec)
		if ok {
			log.Printf("struct name: %s", st.Name.Name)

			won = NewWon()
			won.StructName = st.Name.Name
			won.NameDash, won.NameUnderscore, won.NameCamel, won.NameFirstChar = getVariations(won.StructName)

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
				newJSONField.NameDash, newJSONField.NameUnderscore, newJSONField.NameCamel, newJSONField.NameFirstChar = getVariations(v)
				possibleJSONFields = append(possibleJSONFields, newJSONField)
			}
			log.Printf("possibleJsonFields: %#v", possibleJSONFields)

			saveField.NameDash, saveField.NameUnderscore, saveField.NameCamel, saveField.NameFirstChar = getVariations(saveField.FieldName)

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
						saveField.Enums = possibleJSONFields
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
			if isFK {
				won.ForeignKeyField = append(won.ForeignKeyField, saveField)
			}

			//log.Printf("json fields: %v", won.JSONFields)

			won.StructFields = append(won.StructFields, saveField)
		}
		if won.StructName != "" {
			savedWons = append(savedWons, won)
		}

		return false
	})

	return savedWons
}

func getVariations(oName string) (dash, underscore, camel, firstChar string) {
	oNameParts, err := utils.SplitObjWords(oName)
	if err != nil {
		log.Printf("unabled to splitObjWords for %v:  %v", oName, err)
	}
	return strings.Join(oNameParts, "-"), strings.Join(oNameParts, "_"), lowerCamelJoin(oNameParts), oName[0:1]

}

type Won struct {
	BasePackage    string
	StructName     string
	NameDash       string
	NameUnderscore string
	NameCamel      string
	NameFirstChar  string

	StructFields    []StructField
	PrimaryKeyField StructField
	ForeignKeyField []StructField
	EnumFields      []StructField
	JSONFields      []StructField
	TimeFields      []StructField
	GetDBType       func(StructField, string) string
}

func NewWon() Won {
	w := Won{}
	w.GetDBType = func(sf StructField, suffix string) string {
		switch sf.FieldType {
		case "string":
			return "VARCHAR(128) NOT NULL"
		case "int", "int64":
			return "INT NOT NULL"
		case "float64", "float32":
			return "DOUBLE NOT NULL"
		case "Timestamp", "time.Time":
			return "DATETIME(6) NOT NULL"
		default:
			switch sf.TagValue {
			case "json_struct":
				return `TEXT NOT NULL DEFAULT "{}"`
			case "enum":
				if sf.TagValue2 == "string" {
					return `VARCHAR(128) NOT NULL DEFAULT ""`
				} else {
					return "INT NOT NULL DEFAULT 0"
				}
			}
		}
		return "invalid_type"
	}
	return w

}

type StructField struct {
	FieldName      string
	NameDash       string
	NameUnderscore string
	NameCamel      string
	NameFirstChar  string

	FieldType string
	TagType   string
	TagValue  string
	TagValue2 string
	Enums     []StructField

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
