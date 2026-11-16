package prompts

import "time"

// AnswerWait bounds how long a blocked agent call waits for the user's answer.
const AnswerWait = 10 * time.Minute
