package compute

import (
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// FunctionImages holds preferred and fallback container images for a runtime.
type FunctionImages struct {
	Preferred string
	Fallback  string
}

var runtimeImageMap = map[string]FunctionImages{
	store.LambdaRuntimePython312: {
		Preferred: "public.ecr.aws/lambda/python:3.12",
		Fallback:  "python:3.12-slim",
	},
	store.LambdaRuntimePython311: {
		Preferred: "public.ecr.aws/lambda/python:3.11",
		Fallback:  "python:3.11-slim",
	},
	store.LambdaRuntimeNodejs20x: {
		Preferred: "public.ecr.aws/lambda/nodejs:20",
		Fallback:  "node:20-slim",
	},
}

// PreferredFunctionImage is the AWS Lambda Python 3.12 base (one-shot override, not RIE).
const PreferredFunctionImage = "public.ecr.aws/lambda/python:3.12"

// FallbackFunctionImage is used when the preferred Python 3.12 image cannot be pulled.
const FallbackFunctionImage = "python:3.12-slim"

// FunctionImagesForRuntime returns preferred and fallback images for a lab runtime.
func FunctionImagesForRuntime(runtime string) (FunctionImages, error) {
	imgs, ok := runtimeImageMap[strings.TrimSpace(runtime)]
	if !ok {
		return FunctionImages{}, fmt.Errorf("compute: unsupported runtime %q", runtime)
	}
	return imgs, nil
}

// oneShotNodejs loads HANDLER (file.export), reads the event file, calls the handler once,
// and prints JSON to stdout.
const oneShotNodejs = `
const fs = require("fs");
const handler = process.env.AWS_LAMBDA_FUNCTION_HANDLER || process.env.HANDLER;
if (!handler || handler.indexOf(".") < 0) {
  console.error("handler must be file.export");
  process.exit(1);
}
const dot = handler.lastIndexOf(".");
const modName = handler.slice(0, dot);
const fnName = handler.slice(dot + 1);
const mod = require("/var/task/" + modName);
const fn = mod[fnName];
if (typeof fn !== "function") {
  console.error("handler is not a function");
  process.exit(1);
}
const eventPath = process.env.NOCTAXRIS_EVENT_PATH || "/var/task/.noctaxris-event.json";
const event = JSON.parse(fs.readFileSync(eventPath, "utf8"));
Promise.resolve(fn(event, {}))
  .then((result) => {
    process.stdout.write(JSON.stringify(result) + "\n");
  })
  .catch((err) => {
    console.error(err);
    process.exit(1);
  });
`

const oneShotPythonTemplate = `
import json, os, importlib, sys
for p in ("/opt/python/lib/python%s/site-packages", "/opt/python"):
    if p not in sys.path:
        sys.path.insert(0, p)
handler = os.environ.get("AWS_LAMBDA_FUNCTION_HANDLER") or os.environ["HANDLER"]
if "." not in handler:
    raise SystemExit("handler must be module.function")
mod_name, fn_name = handler.rsplit(".", 1)
sys.path.insert(0, "/var/task")
mod = importlib.import_module(mod_name)
fn = getattr(mod, fn_name)
event_path = os.environ.get("NOCTAXRIS_EVENT_PATH", "/var/task/.noctaxris-event.json")
with open(event_path, "r", encoding="utf-8") as f:
    event = json.load(f)
result = fn(event, None)
print(json.dumps(result))
`

func pythonMinorVersion(runtime string) string {
	switch strings.TrimSpace(runtime) {
	case store.LambdaRuntimePython311:
		return "3.11"
	default:
		return "3.12"
	}
}

func oneShotPythonScript(runtime string) string {
	minor := pythonMinorVersion(runtime)
	return fmt.Sprintf(oneShotPythonTemplate, minor)
}

type oneShotCommand struct {
	Exe    string
	Flag   string
	Script string
}

func zipOneShotCommand(runtime string) (oneShotCommand, error) {
	switch {
	case store.IsPythonLambdaRuntime(runtime):
		return oneShotCommand{
			Exe:    "python",
			Flag:   "-c",
			Script: oneShotPythonScript(runtime),
		}, nil
	case store.IsNodeLambdaRuntime(runtime):
		return oneShotCommand{
			Exe:    "node",
			Flag:   "-e",
			Script: oneShotNodejs,
		}, nil
	default:
		return oneShotCommand{}, fmt.Errorf("compute: unsupported runtime %q", runtime)
	}
}

func imageOneShotCommand(imageURI string) (oneShotCommand, bool) {
	ref := strings.ToLower(strings.TrimSpace(imageURI))
	switch {
	case strings.Contains(ref, "/lambda/python:3.11") || strings.HasPrefix(ref, "python:3.11"):
		return oneShotCommand{Exe: "python", Flag: "-c", Script: oneShotPythonScript(store.LambdaRuntimePython311)}, true
	case strings.Contains(ref, "/lambda/python") || strings.HasPrefix(ref, "python:"):
		return oneShotCommand{Exe: "python", Flag: "-c", Script: oneShotPythonScript(store.LambdaRuntimePython312)}, true
	case strings.Contains(ref, "/lambda/nodejs") || strings.HasPrefix(ref, "node:"):
		return oneShotCommand{Exe: "node", Flag: "-e", Script: oneShotNodejs}, true
	default:
		return oneShotCommand{}, false
	}
}

func zipRuntimeEnv(runtime string) []string {
	switch {
	case store.IsPythonLambdaRuntime(runtime):
		return []string{"PYTHONPATH=/var/task"}
	case store.IsNodeLambdaRuntime(runtime):
		return []string{"NODE_PATH=/var/task:/opt/nodejs/node_modules"}
	default:
		return nil
	}
}
