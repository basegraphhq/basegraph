# Relay's Thinking Layer

Relay is an LLM powered teammate that lives in issue trackers (currently Gitlab, with Github, Linear etc integrations coming soon).

This is a prototype project for Relay 

Relay can
1. Understand the ticket.
2. Has access to explore codebase through graph based retrieval(for code), project's past learnings(postgres query), domain's knowledgebase(postgres query).
3. Spot the gaps in expectations vs what's there. gaps in requirements. gaps in assumptions, gaps in incomplete context. gaps in current implementation, domain edge cases, code edge cases, side effects, impacted areas, pitfalls, things to be careful of.
business edge cases and code edge cases and pitfalls.
4. Offers suggested actions. 
5. Can understand the replies. Based on the reply, it check if this adds new context to our existing context and should we retrieve more context based on reply.
6. At each reply, after fetching updated context, we run Gap detection which is either going to ask follow-up question if gap exists, or marks a gap/concern as closed and updates its learnings about the project.
7. If sufficient amount of Relay's gaps are closed, gap detection finally sends the ticket for Spec generation. Which can be handed off to a human developer or a coding agent. Basically has the entire implementation plan.



Example: 
Title: Add Twilio Support
Description: To enable International calling, we should integrate with Twilio.


Gaps could be:
Requirements:
1. Calling: Inbound, outbound or both?
2. Callbacks: Twilio sends us status callbacks for each state that the call has gone through. But as of now, we don't support incoming webhooks because
the previous discussion (link), we decided that we'll check the call status manually through a async task. To ensure we are not tying ourselves with one dialer.
3. Twilio uses credit systems. If we run out of credits, what kind of notifier are we relying on? Human monitor who can receive emails, grafana alerts?

Devs:
1. What's our rate limit strategy. As of now, we just log it if we receive any other status_code than 200?
    Suggested action: Implement exponential backoff based retries when we hit rate limits. 
    Sources: [@call_tasks.go:410]
2. Twilio offers uLaw as well as mp3. Currently we only support mp3. Should we keep it at that?
    Suggestion: uLaw offers better audio quality. Adding support is not much engineering effort. Do consider, or what do you say?
    Sources: [@exotel.go:164, @audio_utils.go:101]
etc etc.. Just 3-5 important questions to ask
3. As of now, we're not storing which dialer was used to make the call.
    Suggestion: Add a column called 'dialer' to call table 
    Sources: [@init_schema.sql, @add_call_table.sql]


Impact areas to be careful:
1. Our go-based caller microservice does not expect websocket_url in the outbound_call request. 
    Sources: [@core_client.go:12]

Based on the replies, for each reply, we should plan further steps

---

### High level components

Issue Context Service
Can crud on issue_context. 
This is a postgres table that we maintain for a given ticket_id.
Fields
Title, description, members, assignee, reporter, due_date, labels, discussion_thread, code_findings, domain_learnings, project_learnings, keywords, spec, token_cost, retriever_budget


## Planner
#### Planner
**Goal**
Planner receives input, understand's what's missing with our understanding for a given ticket and then plans what contexts needs to be fetched.

**Scope**
1. Tracker event comes in - Source: Tracker
2. Check if the context is sufficient or not.
	1. Sufficient: Call Gap Detector
	2. Insufficient:
		1. Check what all contexts needs to be fetched.
		2. Prepare a focused task for each retriever on what to retrieve.
		3. Calls the Executor
3. Executor comes back with retrieved focus
	1. Source: Executor
	2. Get the budget of retrieval cycles
		1. No Budget: Call the Gap Detector anyways. Ensure that you are passing % confidence about context passed or incomplete context while calling Gap Detector. Gap detector should respond kindly saying that I've looked my best and found these. To explore further, I need more context about Y.
		2. Budget: Run the retrievers based on the updated findings.


**Open questions**
Should Planner tell retrievers what to focus on, or should the retrievers decide themselves? **Planner should set the goal onto what should be searched**

**Nuances**
- Planner should be alloted a budget, ie, run retrievers for these many cycles. If it's still isn't satisfies. It should respond accordingly.
- Planner should have reports from the retriever saying that, data was missing. Unable to find it 

**Edge cases**
- Planner and Executors running infinitely
- Retrievers are unable to fetch due to db issue. No data in DB.
- 

**Callers**: 
- Gitlab Event Webhook
- Executor finishes planned jobs

**Types of events**
- Relay is tagged to a fresh issue
- PM or dev replies on a comment
	- Reply to relay
	- Tags relay to check other's reply
- Executor finishes the planned_jobs

**Calls**:
Gap Detector


## Gap Detector
Gap Detector runs when the planner decides that context is sufficient for a given event.
Gap Detector checks the gaps in the requirements.
