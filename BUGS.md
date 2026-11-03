# Debugging Exercise — Bug Fixes

The debugging program processes campaign events from a JSONL file and produces campaign statistics.

## Bug 1: Duplicate events

**Problem:** The same event can appear more than once in the input file. Counting every occurrence inflates campaign statistics.

**Why it happens:** Event delivery systems may retry requests or send the same event again. Without tracking previously processed event IDs, repeated events can be counted multiple times.

**Fix:** Added a `seenEventIDs` map to track event IDs across the entire input file. An event ID is marked as seen so subsequent occurrences are skipped.

**Verification:** Repeated event IDs are counted only once, including when duplicates appear in different batches.

## Bug 2: Unique opens

**Problem:** Multiple open events from the same contact should count as one unique open for a campaign.

**Why it happens:** Counting every open event as a unique open treats repeated activity from one contact as activity from multiple contacts.

**Fix:** Added an `openedBy` map using the campaign ID and contact ID as a combined key.

**Verification:** Multiple opens from the same contact count as one unique open within that campaign. The same contact can still count as a unique open in another campaign.

## Bug 3: Daily delivery dates

**Problem:** Delivery events must be grouped by their UTC calendar date.

**Why it happens:** Timestamps can contain different timezone offsets. Extracting the date before converting to UTC can place an event in the wrong daily bucket.

**Fix:** Converted each event timestamp to UTC before formatting its date as `YYYY-MM-DD`.

**Verification:** Delivery events are grouped according to their UTC date, regardless of the timezone offset in the original timestamp.

## Bug 4: Concurrent updates and deterministic output

**Problem:** Concurrent event processing can cause unsafe updates to shared campaign counters. Unsorted map iteration can also produce output in different orders between runs.

**Why it happens:** Multiple goroutines accessing shared maps or counters without synchronization can cause data races or inconsistent results. Go map iteration order is not guaranteed.

**Fix:** Protected shared campaign counter updates with a mutex. Sorted campaign names and daily date keys before printing the report.

**Verification:** The race detector reports no data races, and sorting makes report ordering deterministic.

## Verification performed

- Ran `go run . events.jsonl`.
- Compared the output with `expected_output.txt` using `diff`.
- Ran `go run -race . events.jsonl` and checked for race warnings.
- Repeated the execution 10 times and compared every output with `expected_output.txt`.
- All 10 repeated runs matched the expected output.
