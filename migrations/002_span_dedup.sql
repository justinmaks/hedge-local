-- OTLP exporters retry on timeout/5xx. Without a uniqueness guarantee on
-- span_id every retry re-inserted the whole batch, double-counting cost and
-- tokens. Remove existing duplicates (keep the earliest row per span_id) and
-- subtract exactly what those duplicates added to session aggregates, then
-- enforce uniqueness so INSERT OR IGNORE can drop retried spans at write
-- time. Aggregates are adjusted, not recomputed from llm_calls: older hcli
-- versions credited session totals from sources that left no llm_calls rows,
-- and a full recompute would erase that history.

CREATE TEMPORARY TABLE _dup_llm AS
SELECT id, session_id, cost_usd, input_tokens, output_tokens,
       cache_read_tokens, cache_write_tokens
FROM llm_calls
WHERE span_id IS NOT NULL AND span_id != ''
  AND id NOT IN (
    SELECT MIN(id) FROM llm_calls
    WHERE span_id IS NOT NULL AND span_id != ''
    GROUP BY span_id
  );

CREATE TEMPORARY TABLE _dup_tool AS
SELECT id, session_id
FROM tool_calls
WHERE span_id IS NOT NULL AND span_id != ''
  AND id NOT IN (
    SELECT MIN(id) FROM tool_calls
    WHERE span_id IS NOT NULL AND span_id != ''
    GROUP BY span_id
  );

DELETE FROM llm_calls WHERE id IN (SELECT id FROM _dup_llm);
DELETE FROM tool_calls WHERE id IN (SELECT id FROM _dup_tool);

UPDATE sessions SET
  total_cost_usd           = total_cost_usd           - COALESCE((SELECT SUM(cost_usd)           FROM _dup_llm WHERE session_id = sessions.id), 0),
  total_input_tokens       = total_input_tokens       - COALESCE((SELECT SUM(input_tokens)       FROM _dup_llm WHERE session_id = sessions.id), 0),
  total_output_tokens      = total_output_tokens      - COALESCE((SELECT SUM(output_tokens)      FROM _dup_llm WHERE session_id = sessions.id), 0),
  total_cache_read_tokens  = total_cache_read_tokens  - COALESCE((SELECT SUM(cache_read_tokens)  FROM _dup_llm WHERE session_id = sessions.id), 0),
  total_cache_write_tokens = total_cache_write_tokens - COALESCE((SELECT SUM(cache_write_tokens) FROM _dup_llm WHERE session_id = sessions.id), 0),
  message_count            = message_count            - (SELECT COUNT(*) FROM _dup_llm WHERE session_id = sessions.id)
WHERE id IN (SELECT DISTINCT session_id FROM _dup_llm);

UPDATE sessions SET
  tool_call_count = tool_call_count - (SELECT COUNT(*) FROM _dup_tool WHERE session_id = sessions.id)
WHERE id IN (SELECT DISTINCT session_id FROM _dup_tool);

DROP TABLE _dup_llm;
DROP TABLE _dup_tool;

CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_calls_span_unique
  ON llm_calls(span_id) WHERE span_id IS NOT NULL AND span_id != '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_calls_span_unique
  ON tool_calls(span_id) WHERE span_id IS NOT NULL AND span_id != '';
