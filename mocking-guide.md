# MockDock Developer Mocking & Generative AI Guide

Building high-fidelity mock environments is essential for reliable testing and developer productivity. This guide describes how to construct accurate mock payloads and database schemas by leveraging **Generative AI Coding Assistants** (e.g., Gemini, Claude, ChatGPT) in combination with static codebase analysis and dynamic traffic observation.

---

## 1. Introduction: AI-Driven Mocking

Generative AI tools excel at translating raw schemas, type definitions, and network logs into structured JSON mock datasets or server mock scripts. By using structured prompts, you can delegate the tedious work of creating mocks to your AI assistant.

### Prompting Best Practices
*   **Provide Full Context**: Always feed the assistant the exact TypeScript types, database schemas, OpenAPI/Swagger specifications, or raw network log content.
*   **Specify Constraints**: Instruct the AI on format requirements (e.g., "Output strict RFC-compliant JSON", "Create a self-contained Node.js module").
*   **Include Edge Cases**: Specifically prompt the assistant to generate bad request states (`400`), unauthorized attempts (`401`), and server exception mock patterns (`500`).

---

## 2. Strategy A: Codebase Analysis (Static Mocking)

Static mocking analyzes your application's source code files to deduce what endpoints, query arguments, and payload structures are expected.

### Front-End Codebase Analysis
To mock a backend service, you must first inspect how the front-end queries it.
1.  **TypeScript Types**: Locate the interfaces, types, or DTO classes representing API response schemas.
2.  **API Clients**: Inspect your Fetch client, Axios routes, or GraphQL queries to identify the HTTP verbs, paths, and headers expected.

#### Prompt Template: TS Interface to JSON Mock
```text
System: You are an expert backend QA automation engineer.
User: Analyze the following front-end TypeScript interface definitions and generate a realistic mock JSON array containing 5 mock records matching this schema. Be sure to populate optional fields in some records and leave them absent in others:

[PASTE TYPESCRIPT INTERFACES HERE]
```

---

### Back-End Codebase Analysis
If you have access to the backend source code but need to mock a third-party microservice or database dependency:
1.  **Route Controllers**: Check route definitions to identify paths, path variables, query parameters, and expected response payloads.
2.  **Database Schemas**: Inspect Prisma schemas, Mongoose models, or database migrations to capture relationships and entity fields.

#### Prompt Template: Prisma/Mongoose Model to REST API Stub
```text
System: You are an expert systems integrations engineer.
User: Analyze the following Prisma schema definition. Generate:
1. A JSON GET response mock that returns a list of these models, including realistic relational associations.
2. A MockDock JavaScript mock handler function that intercepts POST requests to create a new model, checks for mandatory fields, and responds with a 400 status code if a field is missing, or a 201 status code and the created record on success.

[PASTE PRISMA/MONGOOSE SCHEMA HERE]
```

---

## 3. Strategy B: Payload Analysis (Dynamic Mocking)

Dynamic mocking uses actual traffic logs, network intercepts, or database queries from a running instance of your application.

### Analyzing Network Traffic (HAR & cURL)
Using your web browser's developer console to record live traffic is the easiest way to capture precise mock structures.
1.  Open Chrome/Firefox DevTools (F12) -> Select the **Network** tab.
2.  Interact with the app to trigger the API calls.
3.  Right-click any request in the list and select:
    *   **Copy** -> **Copy as cURL** (for individual requests).
    *   **Save all as HAR with content** (to capture a sequence of requests/responses).

#### Prompt Template: cURL to Mock Endpoint
```text
System: You are an expert security-focused QA engineer.
User: Convert the following cURL request and raw API response payload into a sanitized mock JSON response.
1. Remove all sensitive production credentials, OAuth tokens, cookies, and real user PII (emails, phone numbers, addresses).
2. Replace them with realistic, randomized mock placeholders.
3. Output the final result as a valid JSON mock.

[PASTE cURL AND RESPONSE PAYLOAD HERE]
```

---

### Analyzing Database Queries (SQL Exports)
For database mocking stubs (Postgres, MySQL), capture raw query execution data.
1.  Export a subset of rows from your production or staging database as a SQL insert dump or JSON list.
2.  Capture the DDL definition of the tables.

#### Prompt Template: SQL Table DDL to SQLite Mock Seed
```text
System: You are an expert SQLite database administrator.
User: Analyze the following PostgreSQL DDL schema and raw data sample. Convert the schema definition to SQLite-compatible syntax and generate a set of SQLite INSERT statements to seed a mock SQLite database containing this sample data:

[PASTE POSTGRES DDL AND DATA HERE]
```

---

## 4. Integration with MockDock Stubs

Once you have generated your mock payloads or scripts with your generative AI assistant, configure MockDock to serve them:

### 1. Registering Static Responses
Place your AI-generated JSON payloads into your project's `mockdock/sources/` directory. You can map them to paths in the UI:
*   **Stub Type**: `HTTP`
*   **Response Body**: Paste the generated JSON directly, or load the source file.

### 2. State-Based Dynamic Mocks (JS Interceptors)
If your mock needs to behave dynamically (such as maintaining a local list of mock items), save the AI-generated javascript code as a mock script file in your workspace:
*   **Configuration**: Select the service -> click **Settings** -> **Add JS Interceptor**.
*   **Code Example**:
    ```javascript
    module.exports = function(req, res) {
      // Generated by AI assistant
      if (req.method === 'POST') {
        const { title } = req.body;
        if (!title) return res.status(400).json({ error: "Missing title" });
        return res.status(201).json({ id: Date.now().toString(), title });
      }
      return res.status(200).json({ items: [] });
    };
    ```

### 3. Relational Database Virtualization (SQL-to-SQLite)
For MySQL or Postgres mocking stubs:
1.  Go to the service's stub configuration in the MockDock dashboard.
2.  Enable **SQLite Mapping**.
3.  Paste the SQLite DDL schema and insert statements generated in **Strategy B** into the **Init Script** editor box.
4.  MockDock will spin up a lightweight, containerized sqlite instance and translate incoming native queries in real-time, serving database mocks dynamically.
