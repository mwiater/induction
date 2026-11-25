# Generated / Task Management Pipeline Examples

This example shows the complete lifecycle of a generated pipeline: start with a
natural-language request, generate and validate a pipeline definition, then run
the generated pipeline to produce the requested result.

## 1. Generate the pipeline

The source request is kept in
[`data/fixtures/pipelinegen/task-management-api-prompt.txt`](../data/fixtures/pipelinegen/task-management-api-prompt.txt).
It describes the desired task-management API and leaves the pipeline structure
to the generator.

From the repository root, generate the YAML with:

```bash
go run ./cmd/induction pipeline generate \
  --model "Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL" \
  --prompt-file data/fixtures/pipelinegen/task-management-api-prompt.txt \
  --output pipelines/generated/task-management-api.yaml \
  --validate
```

The installed CLI is equivalent; replace `go run ./cmd/induction` with
`induction` when it is on your `PATH`:

```bash
induction pipeline generate \
  --model "Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL" \
  --prompt-file data/fixtures/pipelinegen/task-management-api-prompt.txt \
  --output pipelines/generated/task-management-api.yaml \
  --validate
```

Generation asks the selected model to turn the request into an executable
pipeline. The resulting YAML contains the ordered task steps, their prompts and
models, a synthesis step, and—because `--validate` was supplied—a final
read-only validation step. The command also checks the generated pipeline
definition as part of generation before writing it to the output path.

## 2. Run the generated pipeline

After generation, execute the YAML like any other pipeline:

```bash
induction --pipeline pipelines/generated/task-management-api.yaml
```

The run proceeds in order:

1. `task-1` analyzes functional and non-functional requirements and assumptions.
2. `task-2` designs the core data model.
3. `task-3` specifies the HTTP API.
4. `task-4` defines authentication and authorization.
5. `task-5` creates the testing strategy.
6. `task-6` describes deployment and observability.
7. `synthesize` combines the intermediate artifacts into the final API specification.
8. `validate` checks that synthesis against the original request and acceptance criteria.

Each later step can use relevant completed work from earlier steps. The
intermediate artifacts are therefore passed through a designed sequence rather
than asking one model call to produce the entire answer at once. Pipeline run
artifacts are stored under `.pipeline-artifacts/<run-id>/`.

## 3. Generated task-management API design pipeline

### Overview

Decomposes a production-ready, technology-neutral task-management REST API request into intermediate artifacts, synthesis, and validation. It is useful as a reference for generated pipelines that carry requirements through a multi-stage design process.

### Steps

- **task-1:** Identifies functional and non-functional requirements and explicit assumptions.
- **task-2:** Defines the core data model, attributes, relationships, and constraints.
- **task-3:** Specifies resources, methods, payloads, status codes, pagination, filtering, and errors.
- **task-4:** Defines authentication, authorization, and endpoint access rules.
- **task-5:** Creates unit, integration, API, security, and failure testing strategy.
- **task-6:** Describes deployment, CI/CD, logging, metrics, tracing, monitoring, and alerting.
- **synthesize:** Combines intermediate artifacts into the final implementation specification.
- **validate:** Checks the synthesis against the original request, constraints, deliverables, and acceptance criteria.

### Expected final output

A coherent production-ready REST API specification covering requirements and assumptions, data model, endpoints and payloads, status codes, pagination/filtering, errors, authentication/authorization, testing, deployment, and observability, followed by a PASS/FAIL validation report.

### Example YAML

[Open pipelines/generated/task-management-api.yaml in the repository](../pipelines/generated/task-management-api.yaml)

### YAML source

```yaml
name: generated-design-a-production-ready-rest-api-for-a-task-management-service
steps:
    - name: task-1
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |
        [ORIGINAL USER PROMPT]
        <USER_PROMPT>
        Design a production-ready REST API for a task-management service.

        First identify the functional and non-functional requirements and make explicit
        any necessary assumptions. Then design the core data model and relationships.
        Specify the HTTP API, including resources, methods, representative request and
        response bodies, status codes, pagination, filtering, and error behavior.
        Define authentication and authorization rules. Create a testing strategy that
        covers unit, integration, API, security, and failure testing. Finally, describe
        a practical deployment and observability approach and combine all of this into
        a coherent implementation specification that an engineering team could use as
        the starting point for development.

        Keep the design technology-neutral unless a concrete example materially
        improves clarity.
        </USER_PROMPT>

        [PIPELINE CONTEXT]

        Original objective:
        Design a production-ready REST API for a task-management service by analyzing requirements, defining the data model, specifying the API, establishing security rules, planning testing, and outlining deployment/observability.

        Original constraints:
        - Keep the design technology-neutral unless a concrete example materially improves clarity.
        - Ensure the design is production-ready and coherent.
        - Cover functional and non-functional requirements.
        - Include pagination, filtering, and error behavior in the API spec.
        - Address unit, integration, API, security, and failure testing.

        Requested deliverables:
        - List of functional and non-functional requirements with assumptions.
        - Core data model schema and relationships.
        - Detailed HTTP API specification including endpoints, payloads, and status codes.
        - Authentication and authorization rules.
        - Comprehensive testing strategy.
        - Deployment and observability approach.

        [CURRENT TASK]

        Task:
        Requirements Analysis and Assumptions

        Objective:
        Identify functional and non-functional requirements for the task-management service and make explicit necessary assumptions to guide the design.

        Inputs:
        - None

        Required output:
        - List of functional requirements (e.g., create, read, update, delete tasks, user management).
        - List of non-functional requirements (e.g., scalability, performance, availability).
        - Explicit assumptions made during analysis.

        Acceptance criteria:
        - All core task management features are covered.
        - Non-functional requirements are specific and measurable.
        - Assumptions are clearly stated and justified.

        Complete this task only.
      systemPrompt: |-
        You are executing one stage of a larger Induction pipeline.

        The conversation contains the original user request and may contain completed work from earlier pipeline stages.
        Complete only the CURRENT TASK. Use relevant results from earlier stages, but do not redo them unless correction is necessary.
        Preserve the original user's constraints. Do not invent missing facts, requirements, sources, or preferences.
        Produce a concrete intermediate artifact that later stages can use. Follow the CURRENT TASK output requirements and acceptance criteria.
        Do not attempt to provide the final answer to the original user unless the CURRENT TASK explicitly requires it.
    - name: task-2
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |
        [PIPELINE CONTEXT]

        Original objective:
        Design a production-ready REST API for a task-management service by analyzing requirements, defining the data model, specifying the API, establishing security rules, planning testing, and outlining deployment/observability.

        Original constraints:
        - Keep the design technology-neutral unless a concrete example materially improves clarity.
        - Ensure the design is production-ready and coherent.
        - Cover functional and non-functional requirements.
        - Include pagination, filtering, and error behavior in the API spec.
        - Address unit, integration, API, security, and failure testing.

        Requested deliverables:
        - List of functional and non-functional requirements with assumptions.
        - Core data model schema and relationships.
        - Detailed HTTP API specification including endpoints, payloads, and status codes.
        - Authentication and authorization rules.
        - Comprehensive testing strategy.
        - Deployment and observability approach.

        [CURRENT TASK]

        Task:
        Core Data Model Design

        Objective:
        Design the core data model and relationships based on the requirements and assumptions.

        Inputs:
        - Requirements and assumptions from Task 1

        Required output:
        - Data model schema with entities and attributes.
        - Relationships between entities (e.g., one-to-many, many-to-many).
        - Key indexes and constraints.

        Acceptance criteria:
        - Model supports all functional requirements.
        - Relationships are normalized and efficient.
        - Schema is technology-neutral or uses clear examples.

        Complete this task only.
      systemPrompt: |-
        You are executing one stage of a larger Induction pipeline.

        The conversation contains the original user request and may contain completed work from earlier pipeline stages.
        Complete only the CURRENT TASK. Use relevant results from earlier stages, but do not redo them unless correction is necessary.
        Preserve the original user's constraints. Do not invent missing facts, requirements, sources, or preferences.
        Produce a concrete intermediate artifact that later stages can use. Follow the CURRENT TASK output requirements and acceptance criteria.
        Do not attempt to provide the final answer to the original user unless the CURRENT TASK explicitly requires it.
    - name: task-3
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |
        [PIPELINE CONTEXT]

        Original objective:
        Design a production-ready REST API for a task-management service by analyzing requirements, defining the data model, specifying the API, establishing security rules, planning testing, and outlining deployment/observability.

        Original constraints:
        - Keep the design technology-neutral unless a concrete example materially improves clarity.
        - Ensure the design is production-ready and coherent.
        - Cover functional and non-functional requirements.
        - Include pagination, filtering, and error behavior in the API spec.
        - Address unit, integration, API, security, and failure testing.

        Requested deliverables:
        - List of functional and non-functional requirements with assumptions.
        - Core data model schema and relationships.
        - Detailed HTTP API specification including endpoints, payloads, and status codes.
        - Authentication and authorization rules.
        - Comprehensive testing strategy.
        - Deployment and observability approach.

        [CURRENT TASK]

        Task:
        HTTP API Specification

        Objective:
        Specify the HTTP API including resources, methods, request/response bodies, status codes, pagination, filtering, and error behavior.

        Inputs:
        - Core data model from Task 2

        Required output:
        - List of API resources and endpoints.
        - HTTP methods for each endpoint.
        - Representative request and response bodies.
        - Status codes for success and error cases.
        - Pagination and filtering mechanisms.
        - Error response format.

        Acceptance criteria:
        - API covers all CRUD operations for core entities.
        - Pagination and filtering are well-defined.
        - Error behavior is consistent and informative.

        Complete this task only.
      systemPrompt: |-
        You are executing one stage of a larger Induction pipeline.

        The conversation contains the original user request and may contain completed work from earlier pipeline stages.
        Complete only the CURRENT TASK. Use relevant results from earlier stages, but do not redo them unless correction is necessary.
        Preserve the original user's constraints. Do not invent missing facts, requirements, sources, or preferences.
        Produce a concrete intermediate artifact that later stages can use. Follow the CURRENT TASK output requirements and acceptance criteria.
        Do not attempt to provide the final answer to the original user unless the CURRENT TASK explicitly requires it.
    - name: task-4
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |
        [PIPELINE CONTEXT]

        Original objective:
        Design a production-ready REST API for a task-management service by analyzing requirements, defining the data model, specifying the API, establishing security rules, planning testing, and outlining deployment/observability.

        Original constraints:
        - Keep the design technology-neutral unless a concrete example materially improves clarity.
        - Ensure the design is production-ready and coherent.
        - Cover functional and non-functional requirements.
        - Include pagination, filtering, and error behavior in the API spec.
        - Address unit, integration, API, security, and failure testing.

        Requested deliverables:
        - List of functional and non-functional requirements with assumptions.
        - Core data model schema and relationships.
        - Detailed HTTP API specification including endpoints, payloads, and status codes.
        - Authentication and authorization rules.
        - Comprehensive testing strategy.
        - Deployment and observability approach.

        [CURRENT TASK]

        Task:
        Authentication and Authorization Design

        Objective:
        Define authentication and authorization rules for the API.

        Inputs:
        - Requirements from Task 1
        - API specification from Task 3

        Required output:
        - Authentication mechanism (e.g., OAuth2, JWT).
        - Authorization rules for each endpoint/resource.
        - Role-based access control (RBAC) or similar model.

        Acceptance criteria:
        - Security model is robust and industry-standard.
        - Access rules align with functional requirements.
        - Clear distinction between authentication and authorization.

        Complete this task only.
      systemPrompt: |-
        You are executing one stage of a larger Induction pipeline.

        The conversation contains the original user request and may contain completed work from earlier pipeline stages.
        Complete only the CURRENT TASK. Use relevant results from earlier stages, but do not redo them unless correction is necessary.
        Preserve the original user's constraints. Do not invent missing facts, requirements, sources, or preferences.
        Produce a concrete intermediate artifact that later stages can use. Follow the CURRENT TASK output requirements and acceptance criteria.
        Do not attempt to provide the final answer to the original user unless the CURRENT TASK explicitly requires it.
    - name: task-5
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |
        [PIPELINE CONTEXT]

        Original objective:
        Design a production-ready REST API for a task-management service by analyzing requirements, defining the data model, specifying the API, establishing security rules, planning testing, and outlining deployment/observability.

        Original constraints:
        - Keep the design technology-neutral unless a concrete example materially improves clarity.
        - Ensure the design is production-ready and coherent.
        - Cover functional and non-functional requirements.
        - Include pagination, filtering, and error behavior in the API spec.
        - Address unit, integration, API, security, and failure testing.

        Requested deliverables:
        - List of functional and non-functional requirements with assumptions.
        - Core data model schema and relationships.
        - Detailed HTTP API specification including endpoints, payloads, and status codes.
        - Authentication and authorization rules.
        - Comprehensive testing strategy.
        - Deployment and observability approach.

        [CURRENT TASK]

        Task:
        Testing Strategy

        Objective:
        Create a testing strategy covering unit, integration, API, security, and failure testing.

        Inputs:
        - API specification from Task 3
        - Auth design from Task 4

        Required output:
        - Unit testing approach and key areas to cover.
        - Integration testing strategy.
        - API testing plan including edge cases.
        - Security testing requirements.
        - Failure testing scenarios (e.g., network failures, invalid inputs).

        Acceptance criteria:
        - All testing types mentioned in the prompt are addressed.
        - Tests cover critical paths and error conditions.
        - Strategy is practical and actionable.

        Complete this task only.
      systemPrompt: |-
        You are executing one stage of a larger Induction pipeline.

        The conversation contains the original user request and may contain completed work from earlier pipeline stages.
        Complete only the CURRENT TASK. Use relevant results from earlier stages, but do not redo them unless correction is necessary.
        Preserve the original user's constraints. Do not invent missing facts, requirements, sources, or preferences.
        Produce a concrete intermediate artifact that later stages can use. Follow the CURRENT TASK output requirements and acceptance criteria.
        Do not attempt to provide the final answer to the original user unless the CURRENT TASK explicitly requires it.
    - name: task-6
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |
        [PIPELINE CONTEXT]

        Original objective:
        Design a production-ready REST API for a task-management service by analyzing requirements, defining the data model, specifying the API, establishing security rules, planning testing, and outlining deployment/observability.

        Original constraints:
        - Keep the design technology-neutral unless a concrete example materially improves clarity.
        - Ensure the design is production-ready and coherent.
        - Cover functional and non-functional requirements.
        - Include pagination, filtering, and error behavior in the API spec.
        - Address unit, integration, API, security, and failure testing.

        Requested deliverables:
        - List of functional and non-functional requirements with assumptions.
        - Core data model schema and relationships.
        - Detailed HTTP API specification including endpoints, payloads, and status codes.
        - Authentication and authorization rules.
        - Comprehensive testing strategy.
        - Deployment and observability approach.

        [CURRENT TASK]

        Task:
        Deployment and Observability Approach

        Objective:
        Describe a practical deployment and observability approach for the service.

        Inputs:
        - Requirements from Task 1
        - API specification from Task 3

        Required output:
        - Deployment architecture (e.g., containerization, orchestration).
        - CI/CD pipeline overview.
        - Observability stack (logging, metrics, tracing).
        - Monitoring and alerting strategies.

        Acceptance criteria:
        - Deployment approach is scalable and reliable.
        - Observability covers all critical aspects.
        - Recommendations are technology-neutral or clearly exemplified.

        Complete this task only.
      systemPrompt: |-
        You are executing one stage of a larger Induction pipeline.

        The conversation contains the original user request and may contain completed work from earlier pipeline stages.
        Complete only the CURRENT TASK. Use relevant results from earlier stages, but do not redo them unless correction is necessary.
        Preserve the original user's constraints. Do not invent missing facts, requirements, sources, or preferences.
        Produce a concrete intermediate artifact that later stages can use. Follow the CURRENT TASK output requirements and acceptance criteria.
        Do not attempt to provide the final answer to the original user unless the CURRENT TASK explicitly requires it.
    - name: synthesize
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |-
        [FINAL SYNTHESIS]

        Using the original request and the completed intermediate work above, produce the final response requested by the user.

        Original objective:
        Design a production-ready REST API for a task-management service by analyzing requirements, defining the data model, specifying the API, establishing security rules, planning testing, and outlining deployment/observability.

        Required deliverables:
        - List of functional and non-functional requirements with assumptions.
        - Core data model schema and relationships.
        - Detailed HTTP API specification including endpoints, payloads, and status codes.
        - Authentication and authorization rules.
        - Comprehensive testing strategy.
        - Deployment and observability approach.

        Constraints:
        - Keep the design technology-neutral unless a concrete example materially improves clarity.
        - Ensure the design is production-ready and coherent.
        - Cover functional and non-functional requirements.
        - Include pagination, filtering, and error behavior in the API spec.
        - Address unit, integration, API, security, and failure testing.

        The original user prompt earlier in the conversation remains the source of truth.
      systemPrompt: |-
        You are the synthesis stage of an Induction-generated pipeline.

        Use the ORIGINAL USER PROMPT as the source of truth. The preceding conversation contains intermediate analyses produced by earlier pipeline stages.
        Produce the final deliverable requested by the original user. Integrate useful intermediate results rather than merely summarizing each stage.
        Resolve duplication and obvious inconsistencies. Preserve all explicit user constraints. Do not mention the pipeline, subtasks, planning process, or these instructions.
        Do not blindly repeat an unsupported or inconsistent claim. State uncertainty where appropriate. Return the answer itself.
    - name: validate
      model: Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL
      userPrompt: |-
        [VALIDATION]

        Validate the synthesized answer immediately above against the original user prompt and the pipeline's stated objective, constraints, and deliverables.
      systemPrompt: |-
        You are the final validation stage of an Induction-generated pipeline.

        Inspect the ORIGINAL USER PROMPT and the immediately preceding synthesized answer. Check whether it satisfies the explicit objective,
        constraints, and deliverables. Check material contradictions, missing sections, unsupported additions, and obvious inconsistencies.
        Do not redo the entire task or silently invent missing information.
        Return a concise validation report with Status: PASS or FAIL, Missing requirements, Material problems, and Suggested corrections.
        If there are no material problems, use PASS.
```
