package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Todo struct {
	Prefix   string
	Suffix   string
	Id       *string
	Filename string
	Line     int
}

func (todo Todo) String() string {
	if todo.Id == nil {
		return fmt.Sprintf("%s:%d: %sTODO: %s\n",
			todo.Filename, todo.Line,
			todo.Prefix, todo.Suffix)
	} else {
		return fmt.Sprintf("%s:%d: %sTODO(%s): %s\n",
			todo.Filename, todo.Line,
			todo.Prefix, *todo.Id, todo.Suffix)
	}
}

func ref_str(x string) *string {
	return &x
}

func LineAsTodo(line string) *Todo {
	// TODO(#1): LineAsTodo does not support reported TODOs
	// TODO(#2): LineAsTodo has false positive result inside of string literals
	unreportedTodo := regexp.MustCompile("^(.*)TODO: (.*)$")
	groups := unreportedTodo.FindStringSubmatch(line)

	if groups != nil {
		return &Todo{
			Prefix:   groups[1],
			Suffix:   groups[2],
			Id:       nil,
			Filename: "",
			Line:     0,
		}
	}

	return nil
}

func TodosOfFile(path string) ([]Todo, error) {
	result := []Todo{}

	file, err := os.Open(path)
	if err != nil {
		return []Todo{}, err
	}
	defer file.Close()

	line := 1
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		todo := LineAsTodo(scanner.Text())
		if todo != nil {
			todo.Filename = path
			todo.Line = line

			result = append(result, *todo)
		}

		line = line + 1
	}

	return result, scanner.Err()
}

func TodosOfDir(dirpath string) ([]Todo, error) {
	result := []Todo{}

	err := filepath.Walk(dirpath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			todos, err := TodosOfFile(path)

			if err != nil {
				return err
			}

			for _, todo := range todos {
				result = append(result, todo)
			}
		}

		return nil
	})

	return result, err
}

func ListSubcommand() {
	// TODO(#3): ListSubcommand doesn't handle error from TodosOfDir
	todos, _ := TodosOfDir(".")

	for _, todo := range todos {
		fmt.Printf("%v", todo)
	}
}

func ReportSubcommand() {
	// TODO(#5): ReportSubcommand is not implemented
	panic("report is not implemented")
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "list":
			ListSubcommand()
		case "report":
			ReportSubcommand()
		default:
			panic(fmt.Sprintf("`%s` unknown command", os.Args[1]))
		}
	} else {
		//TODO (#8) implement a map for options instead of println'ing them all there
		//also, not sure these descriptions are exactly what rexim means by them
		fmt.Printf("snitch [opt]\n\tlist: lists all possible subcommands\n\treport: reports an issue to github\n")
	}
}
