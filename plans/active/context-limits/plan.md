# Request limits and bounded triage input

Add opt-in Config.MaxRequestBytes over the complete encoded JSON request, typed RequestSizeError, and APIError.ErrorType for server context errors. Do not truncate or retry in the library.

Triage retains the first --context-bytes bytes (default 24576), avoids splitting UTF-8 characters, drains remaining stdin without retaining it, and evaluates once. Report input_bytes, evaluated_bytes, and truncated; verdict applies to the evaluated prefix. No pagination or tokenizer dependency.

Preserve HTTP/evaluator/I/O injection. Use RED/GREEN tests for boundaries, malformed errors, input/drain failures, cancellation, and report scope. Run race tests, build, vet, formatting and diff checks before commits; finish with a real PR #8556 pipeline.
