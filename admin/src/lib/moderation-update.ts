import type { Question } from './api';

export function moderationUpdate(question: Question, patch: Partial<Question>) {
  const next = { ...question, ...patch };
  // Interception removes public visibility; explicitly approving a question
  // moves it out of interception. Merely restoring it does not publish it.
  if (patch.is_spam === true) next.is_publish = false;
  else if (patch.is_publish === true) next.is_spam = false;
  return {
    tag_id: next.tag_id, is_hide: next.is_hide,
    is_rainbow: next.is_rainbow, is_archive: next.is_archive,
    is_publish: next.is_publish, is_spam: next.is_spam,
  };
}
