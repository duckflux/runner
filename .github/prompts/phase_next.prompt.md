---
name: phase_next
description: Prompt used manually by the developer
---

Your workspace for this application is "~/runner".

USE as your context:
- ~/spec/README.md: specification source of truth. ALWAYS FOLLOW the SPEC.
- ~/runner/README.md: current implementation reference.
- ~/runner/docs/HISTORY.md: it's your memory. This is where you can understand the rationale behind previous decisions.

The file "~/runner/TASKS.md" contains a list of implementation tasks grouped by phases. Each phase has a status "Pending" or "Complete". Your job is to complete the next "Pending" task in the list.

Now, follow the instructions below.

FIND the next "Pending ⭕️" phase in "~/runner/TASKS.md". This is your current job.

WORK on ALL the tasks in the current job, following the order they are listed. For each task, make the necessary code changes to implement it. After completing a task, CHECKMARK it in the TASKS.md file.

DON'T ASK for user answers. When dealing with ambigous issues, take the simpler approach.

FOLLOW the plan and the specification strictly.

NEVER edit files in the "~/spec" or "~/runtime-js" folder.

After applying the code changes, RUN all the existing tests.

When you're done coding and testing:
- MARK the phase status as "Complete ✅" in the "~/runner/TASKS.md" file if all the tasks in the phase are done.
- CHECKMARK the task list items for the completed phase.

STOP only when you completed and marked the entire phase complete, including all tasks. I therefore authorize automatic executions of any tool and command necessary to complete the task. DO NOT INTERRUPT your work.