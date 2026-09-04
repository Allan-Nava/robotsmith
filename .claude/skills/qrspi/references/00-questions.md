# 00 · Questions — <TASK-ID> <title>

> **PROMPT (fresh session, clean context)**
>
> You are in the **Questions** phase of the QRSPI workflow.
>
> Input: the ticket text below. Nothing else.
>
> Task: generate the clarifying questions that need answering **before** a line of
> code gets written. Do NOT propose solutions. Do NOT explore the codebase.
>
> For each question give:
> - the question, in one line
> - why it is blocking or risky if left unresolved
> - the **default assumption** that will be used if nobody answers
>
> Order by descending risk. Maximum 10 questions — if you have more, the ticket
> needs splitting.
>
> Fill the skeleton below. Output: markdown only, no preamble.
>
> ```
> TICKET:
> <paste the Linear/Jira ticket here>
> ```

The default assumption is what makes this phase non-blocking: work can proceed
without waiting for answers, and the assumptions are on the record.

---

## Ticket

**ID:** <ENG-1234>
**Link:** <url>
**Title:** <title>

<ticket text>

---

## Questions

### Q1 · <question in one line>

- **Risk if unresolved:** <what goes wrong>
- **Default assumption:** <what we assume in order to proceed>
- **Answer:** _(to be filled — human)_

### Q2 · <...>

- **Risk if unresolved:**
- **Default assumption:**
- **Answer:** _(to be filled — human)_

---

## Out of scope

Things the ticket might suggest but that we are **not** doing in this task:

- <...>

---

## Status

- [ ] Questions generated
- [ ] Reviewed by a human
- [ ] Answers collected (or assumptions explicitly accepted)

> ⏭️ Next phase: **Research**. The ticket is **not** passed to Research — only the
> questions and their answers.
