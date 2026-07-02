package tasker

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type TaskContext map[string]any

func ReadTaskContext(task *Task) (TaskContext, error) {
	path := task.ContextFile
	if path == "" {
		path = filepath.Join(task.Path, "context.json")
	}

	payload := TaskContext{}
	if err := readJSON(path, &payload); err != nil {
		if os.IsNotExist(err) {
			return TaskContext{}, nil
		}
		return nil, err
	}
	if payload == nil {
		payload = TaskContext{}
	}
	return payload, nil
}

func WriteTaskContext(task *Task, context TaskContext) error {
	if context == nil {
		context = TaskContext{}
	}

	path := task.ContextFile
	if path == "" {
		path = filepath.Join(task.Path, "context.json")
	}
	return writeJSON(path, context)
}

func TaskContextValue[T any](task *Task, key string, target *T) (bool, error) {
	context, err := ReadTaskContext(task)
	if err != nil {
		return false, err
	}

	value, ok := context[key]
	if !ok {
		return false, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, err
	}
	return true, nil
}
