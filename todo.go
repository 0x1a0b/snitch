package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
)

// Todo contains information about a TODO in the repo
type Todo struct {
	Prefix   string
	Suffix   string
	ID       *string
	Filename string
	Line     int
	Title    string
}

// LogString formats TODO for compilation logging. Format is
// compatible with Emacs compilation mode, so you can easily jump
// between the todos.
func (todo Todo) LogString() string {
	if todo.ID == nil {
		return fmt.Sprintf("%s:%d: %sTODO: %s",
			todo.Filename, todo.Line,
			todo.Prefix, todo.Suffix)
	}

	return fmt.Sprintf("%s:%d: %sTODO(%s): %s",
		todo.Filename, todo.Line,
		todo.Prefix, *todo.ID, todo.Suffix)
}

func (todo Todo) String() string {
	if todo.ID == nil {
		return fmt.Sprintf("%sTODO: %s",
			todo.Prefix, todo.Suffix)
	}

	return fmt.Sprintf("%sTODO(%s): %s",
		todo.Prefix, *todo.ID, todo.Suffix)
}

func (todo Todo) updateToFile(outputFilename string, lineCallback func(int, string) (string, bool)) error {
	inputFile, err := os.Open(todo.Filename)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	outputFile, err := os.Create(outputFilename)
	if err != nil {
		return err
	}
	defer func() {
		cerr := outputFile.Close()
		if cerr != nil {
			err = cerr
		}
	}()

	scanner := bufio.NewScanner(inputFile)
	lineNumber := 1

	for scanner.Scan() {
		line := scanner.Text()

		replace, remove := lineCallback(lineNumber, line)
		if !remove {
			fmt.Fprintln(outputFile, replace)
		}

		lineNumber = lineNumber + 1
	}

	return err
}

func (todo Todo) updateInPlace(lineCallback func(int, string) (string, bool)) error {
	outputFilename := todo.Filename + ".snitch"
	err := todo.updateToFile(outputFilename, lineCallback)
	if err != nil {
		return err
	}

	err = os.Rename(outputFilename, todo.Filename)

	if err != nil {
		return err
	}

	return err
}

// Update updates the file where the Todo is located in-place.
func (todo Todo) Update() error {
	return todo.updateInPlace(func(lineNumber int, line string) (string, bool) {
		if lineNumber == todo.Line {
			return todo.String(), false
		}

		return line, false
	})
}

// Remove removes the Todo from the file where it is located in-place.
func (todo Todo) Remove() error {
	return todo.updateInPlace(func(lineNumber int, line string) (string, bool) {
		if lineNumber == todo.Line {
			return "", true
		}

		return line, false
	})
}

// GitCommit commits the Todo location to the git repo
func (todo Todo) GitCommit(prefix string) error {
	// TODO(#96): there is no way to check that Todo is unreported at compile time
	if todo.ID == nil {
		panic(fmt.Sprintf("Trying to commit an unreported TODO! %v", todo))
	}

	if err := LogCommand(exec.Command("git", "add", todo.Filename)).Run(); err != nil {
		return err
	}

	if err := LogCommand(exec.Command("git", "commit", "-m", fmt.Sprintf("%s TODO(%s)", prefix, *todo.ID))).Run(); err != nil {
		return err
	}

	return nil
}

func lineAsUnreportedTodo(projectConfig ProjectConfig, line string) *Todo {
	unreportedTodo := regexp.MustCompile("^(.*)TODO: (.*)$")
	groups := unreportedTodo.FindStringSubmatch(line)

	if groups != nil {
		prefix := groups[1]
		suffix := groups[2]
		title := projectConfig.Title.Transform(suffix)

		return &Todo{
			Prefix:   prefix,
			Suffix:   suffix,
			ID:       nil,
			Filename: "",
			Line:     0,
			Title:    title,
		}
	}

	return nil
}

func lineAsReportedTodo(projectConfig ProjectConfig, line string) *Todo {
	unreportedTodo := regexp.MustCompile("^(.*)TODO\\((.*)\\): (.*)$")
	groups := unreportedTodo.FindStringSubmatch(line)

	if groups != nil {
		prefix := groups[1]
		suffix := groups[3]
		id := groups[2]
		title := projectConfig.Title.Transform(suffix)

		return &Todo{
			Prefix:   prefix,
			Suffix:   suffix,
			ID:       &id,
			Filename: "",
			Line:     0,
			Title:    title,
		}
	}

	return nil
}

// LineAsTodo constructs a Todo from a string
func LineAsTodo(projectConfig ProjectConfig, line string) *Todo {
	if todo := lineAsUnreportedTodo(projectConfig, line); todo != nil {
		return todo
	}

	if todo := lineAsReportedTodo(projectConfig, line); todo != nil {
		return todo
	}

	return nil
}

// WalkTodosOfFile visits all of the TODOs in a particular file
func WalkTodosOfFile(projectConfig ProjectConfig, path string, visit func(Todo) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	text, _, err := reader.ReadLine()
	for line := 1; err == nil; line = line + 1 {
		todo := LineAsTodo(projectConfig, string(text))

		if todo != nil {
			todo.Filename = path
			todo.Line = line

			if err := visit(*todo); err != nil {
				return err
			}
		}

		text, _, err = reader.ReadLine()
	}

	if err != io.EOF {
		return err
	}

	return nil
}

// WalkTodosOfDir visits all of the TODOs in a particular directory
func WalkTodosOfDir(projectConfig ProjectConfig, dirpath string, visit func(todo Todo) error) error {
	cmd := exec.Command("git", "ls-files", dirpath)
	var outb bytes.Buffer
	cmd.Stdout = &outb

	err := cmd.Run()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(&outb)

	for scanner.Scan() {
		filepath := scanner.Text()
		err = WalkTodosOfFile(projectConfig, filepath, visit)
		if err != nil {
			return err
		}
	}

	return err
}

func queryGithubAPI(creds GithubCredentials, method, url string, jsonBody map[string]interface{}) (map[string]interface{}, error) {
	client := &http.Client{}

	bodyBuffer := new(bytes.Buffer)
	err := json.NewEncoder(bodyBuffer).Encode(jsonBody)

	req, err := http.NewRequest(
		method, url, bodyBuffer)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "token "+creds.PersonalToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var v map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}

	return v, err
}

// RetrieveGithubStatus retrieves the current status of TODOs issue
// from GitHub
func (todo Todo) RetrieveGithubStatus(creds GithubCredentials, repo string) (string, error) {
	json, err := queryGithubAPI(
		creds,
		"GET",
		// TODO(#59): possible GitHub API injection attack
		"https://api.github.com/repos/"+repo+"/issues/"+(*todo.ID)[1:],
		nil)

	if err != nil {
		return "", err
	}

	return json["state"].(string), nil
}

// ReportTodo reports the todo as a Github Issue, updates the file
// where the todo is located and commits the changes to the git repo.
func (todo Todo) ReportTodo(creds GithubCredentials, repo string, body string) (Todo, error) {
	// TODO(#60): ReportTodo is not a Todo method
	json, err := queryGithubAPI(
		creds,
		"POST",
		"https://api.github.com/repos/"+repo+"/issues",
		map[string]interface{}{
			"title": todo.Title,
			"body":  body,
		})

	id := "#" + strconv.Itoa(int(json["number"].(float64)))
	todo.ID = &id

	return todo, err
}
