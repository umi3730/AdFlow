import type { Decision, TargetingExplanation } from './api';

export function groupTargetingCandidates(
  report: TargetingExplanation,
  decision: Pick<Decision, 'matched' | 'campaignId'>,
) {
  const winnerId = decision.matched ? decision.campaignId : null;
  const winner = winnerId
    ? report.candidates.find((candidate) => candidate.campaignId === winnerId)
    : undefined;
  const others = winnerId
    ? report.candidates.filter((candidate) => candidate.campaignId !== winnerId)
    : report.candidates;
  return { winner, others };
}
