const demoNames = new Map([
  ['demo-auction-user', '竞价演示用户'],
  ['demo-user-match', '基础投放用户'],
  ['demo-user-excluded', '排除规则用户'],
  ['demo-user-miss', '兴趣对照用户'],
]);

// A display alias describes the bundled example, not its current eligibility.
// Custom IDs, including case and spelling, stay unchanged.
export function profileDisplayName(userId: string): string {
  return demoNames.get(userId) ?? userId;
}

export function isDemoProfile(userId: string): boolean {
  return demoNames.has(userId);
}
