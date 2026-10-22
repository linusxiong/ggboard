/** Tabs of `/admin/rulesets` (kept dependency-free: the route's `validateSearch` imports it eagerly). */
export const RULESET_TABS = ['sources', 'categories', 'templates', 'mirrors', 'custom'] as const
export type RulesetTab = (typeof RULESET_TABS)[number]
