# Query Executor Slack Bot

A Slack bot that accepts MongoDB read queries in a dedicated channel and executes them only after an authorized approver reacts with the configured emoji.

<img width="1500" height="900" alt="output" src="https://github.com/user-attachments/assets/9f9f809e-9ae1-41dc-9d2f-9a29c1048470" />


## What It Does

- Accepts shell-style MongoDB queries from Slack messages
- Stores each query as pending until approval
- Verifies approver membership in a Slack user group
- Executes only safe read operations
- Uploads results as a JSON file in the original message thread

## Installation

### 1. Prerequisites

- Go 1.25+
- A Slack app configured for Socket Mode
- MongoDB connection details

### 2. Clone and enter the project

    git clone <your-repo-url>
    cd slack-query-executor

### 3. Set environment variables

Create a local env file and set the required values.

Required variables:

- SLACK_BOT_TOKEN
- SLACK_APP_TOKEN
- SLACK_CHANNEL_ID
- SLACK_APPROVER_GROUP_ID
- MONGO_URI
- MONGO_DB_NAME

Optional:

- APPROVE_EMOJI (default: white_check_mark)

Example:

    cp .env.example .env

Then edit .env and fill in real values.

### 4. Build and run

    go build -o bin/bot ./cmd/bot
    ./bin/bot

### 5. Run tests

    go test ./...

## Run with Docker

Build image:

    docker build -t slack-query-executor-bot .

Run container:

    docker run --rm --env-file .env slack-query-executor-bot

Docker Hub:

https://hub.docker.com/r/devsteam/slack-query-executor

## Needed Slack Permissions

### App-Level Token

Create an app-level token for Socket Mode with this scope:

- connections:write

### Bot Token Scopes

Add these bot scopes in Slack OAuth and Permissions:

- channels:history
- reactions:read
- usergroups:read
- files:write
- chat:write

### Event Subscriptions

Enable these bot events:

- message.channels
- reaction_added
- app_mention

### Channel Setup

- Invite the bot user to the target channel
- Set SLACK_CHANNEL_ID to that channel

## Slack Link Transformation Caveats

Slack may rewrite values in message text before the bot receives them.

- Email values can be transformed to rich-text form:
    - andrei@some.com
    - becomes <mailto:andrei@some.com|andrei@some.com>
- The parser normalizes mailto wrappers back to plain email text.
- Slack can also add rich-text wrappers to URLs (for example: <https://example.com|example.com>).

Because query parsing requires strict JSON:

- Always wrap queries in triple backticks.
- Keep keys and string values double-quoted.
- If an exact string match is important (especially for URL fields), verify what Slack sent in the code block.

## Notes

- The bot only listens in one configured channel
- The bot ignores its own messages and unsupported subtypes
- Query execution is gated by emoji approval from an authorized user group
