#!/usr/bin/env python3
"""Count tokens for Claude and GPT-4o from an input file.

Usage: python count_tokens.py <input_file>
Stdout: {"claude_tokens": N, "gpt4o_tokens": M, "file": "<path>"}
"""

import json
import sys

def main():
    if len(sys.argv) != 2:
        json.dump({"error": "Usage: python count_tokens.py <input_file>"}, sys.stdout)
        print()
        sys.exit(1)

    input_file = sys.argv[1]

    try:
        with open(input_file, encoding="utf-8") as f:
            text = f.read()
    except FileNotFoundError:
        json.dump({"error": f"File not found: {input_file}"}, sys.stdout)
        print()
        sys.exit(1)
    except Exception as e:
        json.dump({"error": str(e)}, sys.stdout)
        print()
        sys.exit(1)

    # GPT-4o token count using tiktoken cl100k_base
    import tiktoken
    enc = tiktoken.get_encoding("cl100k_base")
    gpt4o_tokens = len(enc.encode(text))

    # Claude token count
    result = {"gpt4o_tokens": gpt4o_tokens, "file": input_file}

    try:
        from anthropic import Anthropic
        client = Anthropic()
        # Use the SDK's token counting if available
        count_resp = client.messages.count_tokens(
            model="claude-sonnet-4-20250514",
            messages=[{"role": "user", "content": text}],
        )
        result["claude_tokens"] = count_resp.input_tokens
    except Exception:
        # Fallback: use cl100k_base as approximation for Claude
        result["claude_tokens_approx"] = gpt4o_tokens

    json.dump(result, sys.stdout)
    print()


if __name__ == "__main__":
    main()
