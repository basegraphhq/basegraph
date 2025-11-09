# Test Prompts for Parallel Tool Calling Verification

## Test Prompt 1: Multiple Symbol Searches (Recommended)
This prompt should trigger 3-4 parallel `search_code_symbols` calls:

```
Search for all functions, structs, and interfaces that contain "extract" in their name. Also search for anything with "neo4j" in the name, and anything with "tool" in the name. Give me a summary of what you find.
```

**Expected behavior:**
- Multiple `search_code_symbols` tool calls should be made in parallel
- Check the console output: you should see multiple "🔧 Calling tool: search_code_symbols" messages appear quickly
- The tool execution times should be similar (indicating parallel execution), not sequential
- Total time should be roughly equal to the longest tool call, not the sum of all calls

---

## Test Prompt 2: Mixed Tool Types
This prompt should trigger parallel calls across different tool types:

```
Search for functions named "Run" and also search for structs containing "Config". Then get details for the first result from each search, and also read the file assistant/runner.go to see the implementation.
```

**Expected behavior:**
- Should trigger: `search_code_symbols` (x2), `get_symbol_details` (x2), and `read_entire_file` (x1) in parallel
- All 5 tools should execute concurrently
- Console should show all tool calls starting before any complete

---

## Test Prompt 3: Multiple Symbol Details
This prompt should trigger multiple `get_symbol_details` calls:

```
Get detailed information about the following symbols: search_code_symbols, get_symbol_details, and grep_code_nodes. Include their relationships.
```

**Expected behavior:**
- 3 `get_symbol_details` calls should execute in parallel
- Each call should fetch relationships independently
- Execution times should overlap (parallel), not be sequential

---

## Test Prompt 4: Complex Multi-Tool Query
This prompt should trigger the most parallel calls:

```
I need to understand the codebase structure. Please:
1. Search for all interfaces in the codebase
2. Search for all structs containing "Tool"
3. Search for all functions with "Handle" in the name
4. Get details for the ToolRegistry struct
5. Read the assistant/runner.go file
6. List the assistant directory

Give me a comprehensive overview.
```

**Expected behavior:**
- Should trigger 6+ tool calls in parallel
- Console output should show all tools starting execution before any complete
- Total response time should be close to the slowest tool, not the sum

---

## What to Look For When Testing

### ✅ Signs of Correct Parallel Execution:
1. **Console Output Pattern:**
   ```
   🔧 Calling tool: search_code_symbols
      Input: {...}
   🔧 Calling tool: search_code_symbols
      Input: {...}
   🔧 Calling tool: get_symbol_details
      Input: {...}
   [All appear quickly, before any completion]
   ⏱️  Tool execution time (search_code_symbols): 150ms
   ⏱️  Tool execution time (search_code_symbols): 180ms
   ⏱️  Tool execution time (get_symbol_details): 200ms
   ```

2. **Timing Verification:**
   - If 3 tools each take ~200ms, total should be ~200ms (parallel)
   - NOT ~600ms (sequential)
   - Check the "⏱️ Response time" - it should reflect parallel execution

3. **Call ID Matching:**
   - Each tool call should have a unique `call_id`
   - Each output should match its corresponding call ID
   - No mismatched results

### ❌ Signs of Sequential Execution (Bug):
- Tools complete one at a time
- Total time = sum of individual tool times
- Console shows: tool1 starts → tool1 completes → tool2 starts → tool2 completes

---

## Quick Verification Command

After running any test prompt, check the conversation log file in `.conversations/` directory. The JSON should show:
- All `function_call` items grouped together
- All `function_output` items grouped together after the calls
- Each output's `call_id` matching its corresponding call

