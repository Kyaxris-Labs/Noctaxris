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
	store.LambdaRuntimePython313: {
		Preferred: "public.ecr.aws/lambda/python:3.13",
		Fallback:  "python:3.13-slim",
	},
	store.LambdaRuntimePython314: {
		Preferred: "public.ecr.aws/lambda/python:3.14",
		Fallback:  "python:3.14-slim",
	},
	store.LambdaRuntimeNodejs20x: {
		Preferred: "public.ecr.aws/lambda/nodejs:20",
		Fallback:  "node:20-slim",
	},
	store.LambdaRuntimeNodejs22x: {
		Preferred: "public.ecr.aws/lambda/nodejs:22",
		Fallback:  "node:22-slim",
	},
	store.LambdaRuntimeNodejs24x: {
		Preferred: "public.ecr.aws/lambda/nodejs:24",
		Fallback:  "node:24-slim",
	},
	// Zip one-shot compiles a reflection bootstrap with javac. Prefer Temurin JDK
	// (javac present); fall back to AWS Lambda Java bases when the JDK pull fails.
	store.LambdaRuntimeJava21: {
		Preferred: "eclipse-temurin:21-jdk",
		Fallback:  "public.ecr.aws/lambda/java:21",
	},
	store.LambdaRuntimeJava25: {
		Preferred: "eclipse-temurin:25-jdk",
		Fallback:  "public.ecr.aws/lambda/java:25",
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

// oneShotEcho* run when Image CreateFunction omitted Handler (AWS-faithful).
// Pinned public bases have no /var/task app code; lab smoke needs a deterministic payload.
const oneShotEchoPython = `
import json, os
event_path = os.environ.get("NOCTAXRIS_EVENT_PATH", "/var/task/.noctaxris-event.json")
with open(event_path, "r", encoding="utf-8") as f:
    event = json.load(f)
print(json.dumps({"ok": True, "echo": event}))
`

const oneShotEchoNodejs = `
const fs = require("fs");
const eventPath = process.env.NOCTAXRIS_EVENT_PATH || "/var/task/.noctaxris-event.json";
const event = JSON.parse(fs.readFileSync(eventPath, "utf8"));
process.stdout.write(JSON.stringify({ ok: true, echo: event }) + "\n");
`

const oneShotEchoJavaShell = `set -eu
printf '%s\n' '{"ok":true,"echo":{}}'
`

// oneShotJavaShell writes a reflection bootstrap, compiles it with javac (needs JDK),
// and invokes HANDLER as package.Class::method (default method handleRequest).
// Lab smoke: public static String handleRequest(String in) { return in; }
const oneShotJavaShell = `set -eu
if ! command -v javac >/dev/null 2>&1; then
  echo "noctaxris: javac not found in image (need JDK for Java zip one-shot)" >&2
  exit 1
fi
cat > /tmp/NoctaxrisInvoke.java <<'JAVA'
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;

public class NoctaxrisInvoke {
  public static void main(String[] args) throws Exception {
    String handler = System.getenv("AWS_LAMBDA_FUNCTION_HANDLER");
    if (handler == null || handler.isEmpty()) {
      handler = System.getenv("HANDLER");
    }
    if (handler == null || handler.isEmpty()) {
      System.err.println("handler required (package.Class::method or package.Class)");
      System.exit(1);
    }
    String className;
    String methodName = "handleRequest";
    int sep = handler.indexOf("::");
    if (sep >= 0) {
      className = handler.substring(0, sep).trim();
      String m = handler.substring(sep + 2).trim();
      if (!m.isEmpty()) {
        methodName = m;
      }
    } else {
      className = handler.trim();
    }
    if (className.isEmpty() || !className.contains(".")) {
      System.err.println("handler must be package.Class[::method]");
      System.exit(1);
    }
    String eventPath = System.getenv("NOCTAXRIS_EVENT_PATH");
    if (eventPath == null || eventPath.isEmpty()) {
      eventPath = "/var/task/.noctaxris-event.json";
    }
    String event = Files.readString(Path.of(eventPath), StandardCharsets.UTF_8);
    Class<?> cls = Class.forName(className);
    Method method = findMethod(cls, methodName);
    Object[] params = buildParams(method, event);
    Object target = null;
    if (!Modifier.isStatic(method.getModifiers())) {
      target = cls.getDeclaredConstructor().newInstance();
    }
    Object result = method.invoke(target, params);
    if (result == null) {
      System.out.println("null");
    } else if (result instanceof String) {
      System.out.println((String) result);
    } else {
      System.out.println(String.valueOf(result));
    }
  }

  private static Method findMethod(Class<?> cls, String name) throws NoSuchMethodException {
    Method stringOne = null;
    Method any = null;
    for (Method m : cls.getMethods()) {
      if (!m.getName().equals(name)) {
        continue;
      }
      Class<?>[] pts = m.getParameterTypes();
      if (pts.length == 1 && pts[0] == String.class) {
        return m;
      }
      if (pts.length == 1 && stringOne == null) {
        stringOne = m;
      }
      if (pts.length <= 2 && any == null) {
        any = m;
      }
    }
    if (stringOne != null) {
      return stringOne;
    }
    if (any != null) {
      return any;
    }
    throw new NoSuchMethodException(cls.getName() + "." + name);
  }

  private static Object[] buildParams(Method method, String event) {
    Class<?>[] pts = method.getParameterTypes();
    Object[] params = new Object[pts.length];
    for (int i = 0; i < pts.length; i++) {
      if (pts[i] == String.class || pts[i] == Object.class || pts[i] == CharSequence.class) {
        params[i] = event;
      } else {
        params[i] = null;
      }
    }
    return params;
  }
}
JAVA
javac -cp '/var/task/*:/var/task' /tmp/NoctaxrisInvoke.java
exec java -cp '/tmp:/var/task/*:/var/task' NoctaxrisInvoke
`

func pythonMinorVersion(runtime string) string {
	switch strings.TrimSpace(runtime) {
	case store.LambdaRuntimePython311:
		return "3.11"
	case store.LambdaRuntimePython313:
		return "3.13"
	case store.LambdaRuntimePython314:
		return "3.14"
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
	case store.IsJavaLambdaRuntime(runtime):
		return oneShotCommand{
			Exe:    "/bin/sh",
			Flag:   "-c",
			Script: oneShotJavaShell,
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
	case strings.Contains(ref, "/lambda/python:3.13") || strings.HasPrefix(ref, "python:3.13"):
		return oneShotCommand{Exe: "python", Flag: "-c", Script: oneShotPythonScript(store.LambdaRuntimePython313)}, true
	case strings.Contains(ref, "/lambda/python:3.14") || strings.HasPrefix(ref, "python:3.14"):
		return oneShotCommand{Exe: "python", Flag: "-c", Script: oneShotPythonScript(store.LambdaRuntimePython314)}, true
	case strings.Contains(ref, "/lambda/python") || strings.HasPrefix(ref, "python:"):
		return oneShotCommand{Exe: "python", Flag: "-c", Script: oneShotPythonScript(store.LambdaRuntimePython312)}, true
	case strings.Contains(ref, "/lambda/nodejs") || strings.HasPrefix(ref, "node:"):
		return oneShotCommand{Exe: "node", Flag: "-e", Script: oneShotNodejs}, true
	case strings.Contains(ref, "/lambda/java") || strings.HasPrefix(ref, "eclipse-temurin:"):
		return oneShotCommand{Exe: "/bin/sh", Flag: "-c", Script: oneShotJavaShell}, true
	default:
		return oneShotCommand{}, false
	}
}

// resolveImageOneShot picks the handler import one-shot, or a lab echo one-shot when
// Handler was omitted on CreateFunction (AWS Image packaging).
func resolveImageOneShot(imageURI, handler string) (oneShotCommand, bool) {
	cmd, ok := imageOneShotCommand(imageURI)
	if !ok {
		return oneShotCommand{}, false
	}
	if strings.TrimSpace(handler) != "" {
		return cmd, true
	}
	switch cmd.Exe {
	case "python":
		cmd.Script = oneShotEchoPython
	case "node":
		cmd.Script = oneShotEchoNodejs
	case "/bin/sh":
		cmd.Script = oneShotEchoJavaShell
	default:
		return oneShotCommand{}, false
	}
	return cmd, true
}

func zipRuntimeEnv(runtime string) []string {
	switch {
	case store.IsPythonLambdaRuntime(runtime):
		return []string{"PYTHONPATH=/var/task"}
	case store.IsNodeLambdaRuntime(runtime):
		return []string{"NODE_PATH=/var/task:/opt/nodejs/node_modules"}
	case store.IsJavaLambdaRuntime(runtime):
		return []string{"LAMBDA_TASK_ROOT=/var/task"}
	default:
		return nil
	}
}
