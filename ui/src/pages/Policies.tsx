import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  Card,
  CardBody,
  CardTitle,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  Flex,
  FlexItem,
  FormGroup,
  Label,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  NumberInput,
  PageSection,
  Content,
  Select,
  SelectList,
  SelectOption,
  Switch,
  TextArea,
  TextInput,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { TrashIcon, PlusCircleIcon } from '@patternfly/react-icons';
import { useNavigate } from 'react-router-dom';
import {
  policies,
  providers as providersApi,
  environments as environmentsApi,
  types as typesApi,
  type PolicyRecord,
  type PolicyRule,
} from '../api/client';

// --- Defaults ---

const RESOURCE_TYPES = ['container', 'vm', 'ip', 'dns', 'postgres', 'redis', 'static-site', 'network', 'storage'];
const STRATEGIES = ['first', 'round-robin', 'random', 'cheapest', 'least-loaded', 'bin-pack'];

function emptyRule(): RuleDraft {
  return {
    id: `rule-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
    name: '',
    priority: 0,
    matchType: '',
    matchLabels: '',
    matchExpression: '',
    required: '',
    preferred: [],
    forbidden: [],
    strategy: '',
    properties: '',
    expanded: true,
  };
}

interface RuleDraft {
  id: string;
  name: string;
  priority: number;
  matchType: string;
  matchLabels: string;
  matchExpression: string;
  required: string;
  preferred: string[];
  forbidden: string[];
  strategy: string;
  properties: string;
  expanded: boolean;
}

function ruleToPolicy(draft: RuleDraft): PolicyRule {
  const rule: PolicyRule = {
    match: {},
    providers: {},
  };
  if (draft.name) rule.name = draft.name;
  if (draft.priority) rule.priority = draft.priority;
  if (draft.matchType) rule.match.type = draft.matchType;
  if (draft.matchLabels.trim()) {
    try { rule.match.labels = JSON.parse(draft.matchLabels); } catch { /* ignore */ }
  }
  if (draft.matchExpression.trim()) rule.match.expression = draft.matchExpression;
  if (draft.required) rule.providers.required = draft.required;
  if (draft.preferred.length > 0) rule.providers.preferred = draft.preferred;
  if (draft.forbidden.length > 0) rule.providers.forbidden = draft.forbidden;
  if (draft.strategy) rule.providers.strategy = draft.strategy;
  if (draft.properties.trim()) {
    try { rule.properties = JSON.parse(draft.properties); } catch { /* ignore */ }
  }
  return rule;
}

function policyToRuleDraft(rule: PolicyRule): RuleDraft {
  return {
    id: `rule-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
    name: rule.name || '',
    priority: rule.priority || 0,
    matchType: rule.match.type || '',
    matchLabels: rule.match.labels && Object.keys(rule.match.labels).length > 0 ? JSON.stringify(rule.match.labels) : '',
    matchExpression: rule.match.expression || '',
    required: rule.providers.required || '',
    preferred: rule.providers.preferred || [],
    forbidden: rule.providers.forbidden || [],
    strategy: rule.providers.strategy || '',
    properties: rule.properties && Object.keys(rule.properties).length > 0 ? JSON.stringify(rule.properties, null, 2) : '',
    expanded: false,
  };
}

// --- Main page ---

export default function Policies() {
  const [list, setList] = useState<PolicyRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [isCreateOpen, setCreateOpen] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<PolicyRecord | null>(null);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const load = useCallback(() => {
    setLoading(true);
    policies.list().then(setList).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete placement rule "${name}"?`)) return;
    try {
      await policies.delete(name);
      load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Placement Rules</Content>
      </PageSection>
      <PageSection>
      {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <Button onClick={() => setCreateOpen(true)}>Create rule</Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>
      {!loading && list.length === 0 ? (
        <EmptyState>
          <EmptyStateBody>No placement rules defined.</EmptyStateBody>
          <EmptyStateFooter>
            <EmptyStateActions>
              <Button onClick={() => setCreateOpen(true)}>Create rule</Button>
            </EmptyStateActions>
          </EmptyStateFooter>
        </EmptyState>
      ) : (
        <Table aria-label="Policies">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Rules</Th>
              <Th>Created</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {list.map(p => (
              <Tr key={p.name} onRowClick={() => navigate(`/placement-rules/${p.name}`)} isClickable>
                <Td dataLabel="Name">{p.name}</Td>
                <Td dataLabel="Rules">{p.rules.length}</Td>
                <Td dataLabel="Created">{new Date(p.createdAt).toLocaleString()}</Td>
                <Td dataLabel="Actions" isActionCell>
                  <Button variant="secondary" size="sm" onClick={e => { e.stopPropagation(); setEditingPolicy(p); }} style={{ marginRight: 8 }}>
                    Edit
                  </Button>
                  <Button variant="danger" size="sm" onClick={e => { e.stopPropagation(); handleDelete(p.name); }}>
                    Delete
                  </Button>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
      <PolicyFormModal
        mode="create"
        isOpen={isCreateOpen}
        onClose={() => setCreateOpen(false)}
        onSaved={load}
      />
      <PolicyFormModal
        mode="edit"
        isOpen={!!editingPolicy}
        policy={editingPolicy ?? undefined}
        onClose={() => setEditingPolicy(null)}
        onSaved={load}
      />
    </PageSection>
    </>
  );
}

// --- Shared create/edit modal ---

export function PolicyFormModal({
  mode,
  isOpen,
  policy,
  onClose,
  onSaved,
}: {
  mode: 'create' | 'edit';
  isOpen: boolean;
  policy?: PolicyRecord;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState('');
  const [rules, setRules] = useState<RuleDraft[]>([]);
  const [jsonMode, setJsonMode] = useState(false);
  const [jsonStr, setJsonStr] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [providerNames, setProviderNames] = useState<string[]>([]);
  const [envNames, setEnvNames] = useState<string[]>([]);
  const [typeNames, setTypeNames] = useState<string[]>([]);

  // Load providers, environments, and types for dropdowns
  useEffect(() => {
    if (isOpen) {
      providersApi.list().then(ps => setProviderNames(ps.map(p => p.name))).catch(() => {});
      environmentsApi.list().then(es => setEnvNames(es.filter(e => e.status === 'active').map(e => e.name))).catch(() => {});
      typesApi.list().then(ts => setTypeNames(ts.map(t => t.type))).catch(() => {});
    }
  }, [isOpen]);

  // Initialize form when opening
  useEffect(() => {
    if (!isOpen) return;
    setError('');
    if (mode === 'edit' && policy) {
      setName(policy.name);
      const drafts = policy.rules.map(policyToRuleDraft);
      if (drafts.length > 0) drafts[0].expanded = true;
      setRules(drafts);
      setJsonStr(JSON.stringify(policy.rules, null, 2));
    } else {
      setName('');
      setRules([{ ...emptyRule(), expanded: true }]);
      setJsonStr('[]');
    }
    setJsonMode(false);
  }, [isOpen, mode, policy]);

  // Sync from form to JSON when switching to JSON mode
  const switchToJson = () => {
    const policyRules = rules.map(ruleToPolicy);
    setJsonStr(JSON.stringify(policyRules, null, 2));
    setJsonMode(true);
  };

  // Sync from JSON to form when switching to form mode
  const switchToForm = () => {
    try {
      const parsed: PolicyRule[] = JSON.parse(jsonStr);
      const drafts = parsed.map(policyToRuleDraft);
      if (drafts.length > 0) drafts[0].expanded = true;
      setRules(drafts);
      setJsonMode(false);
    } catch (e) {
      setError('Invalid JSON — fix before switching to form view');
    }
  };

  const addRule = () => {
    setRules(prev => [...prev.map(r => ({ ...r, expanded: false })), { ...emptyRule(), expanded: true }]);
  };

  const removeRule = (idx: number) => {
    setRules(prev => prev.filter((_, i) => i !== idx));
  };

  const updateRule = (idx: number, updates: Partial<RuleDraft>) => {
    setRules(prev => prev.map((r, i) => i === idx ? { ...r, ...updates } : r));
  };

  const toggleExpand = (idx: number) => {
    setRules(prev => prev.map((r, i) => i === idx ? { ...r, expanded: !r.expanded } : r));
  };

  const handleSubmit = async () => {
    setError('');
    setSubmitting(true);
    try {
      let policyRules: PolicyRule[];
      if (jsonMode) {
        policyRules = JSON.parse(jsonStr);
      } else {
        policyRules = rules.map(ruleToPolicy);
      }
      if (mode === 'create') {
        await policies.create({ name, rules: policyRules });
      } else if (policy) {
        await policies.update(policy.name, { rules: policyRules });
      }
      onClose();
      onSaved();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const allProviders = providerNames.length > 0 ? providerNames : ['mock', 'kubernetes'];
  const allEnvironments = envNames;
  const allTypes = typeNames.length > 0 ? typeNames : RESOURCE_TYPES;
  // Combine providers and environments for selection dropdowns
  const allTargets = [...new Set([...allEnvironments, ...allProviders])];

  return (
    <Modal isOpen={isOpen} onClose={onClose} variant="large">
      <ModalHeader title={mode === 'create' ? 'Create Placement Rule' : `Edit Placement Rule: ${policy?.name ?? ''}`} />
      <ModalBody>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}

        <FormGroup label="Name" isRequired={mode === 'create'} fieldId="policy-name" style={{ marginBottom: 16 }}>
          <TextInput id="policy-name" value={name} onChange={(_e, v) => setName(v)} placeholder="production-placement" isDisabled={mode === 'edit'} />
        </FormGroup>

        <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }} style={{ marginBottom: 16 }}>
          <FlexItem>
            <Content component="h3" style={{ margin: 0 }}>Rules ({jsonMode ? 'JSON' : rules.length})</Content>
          </FlexItem>
          <FlexItem>
            <Flex gap={{ default: 'gapSm' }}>
              {!jsonMode && (
                <FlexItem>
                  <Button variant="secondary" icon={<PlusCircleIcon />} size="sm" onClick={addRule}>
                    Add rule
                  </Button>
                </FlexItem>
              )}
              <FlexItem>
                <Switch
                  id="json-toggle"
                  label="JSON"
                  isChecked={jsonMode}
                  onChange={() => jsonMode ? switchToForm() : switchToJson()}
                  isReversed
                />
              </FlexItem>
            </Flex>
          </FlexItem>
        </Flex>

        {jsonMode ? (
          <TextArea
            id="policy-rules-json"
            value={jsonStr}
            onChange={(_e, v) => setJsonStr(v)}
            rows={20}
            style={{ fontFamily: 'monospace', fontSize: 13 }}
          />
        ) : (
          <Flex direction={{ default: 'column' }} gap={{ default: 'gapMd' }}>
            {rules.length === 0 && (
              <FlexItem>
                <Content component="p" style={{ color: '#6a6e73' }}>No rules. Click "Add rule" to get started.</Content>
              </FlexItem>
            )}
            {rules.map((rule, idx) => (
              <FlexItem key={rule.id}>
                <RuleEditor
                  rule={rule}
                  index={idx}
                  allTargets={allTargets}
                  allEnvironments={allEnvironments}
                  allProviders={allProviders}
                  allTypes={allTypes}
                  onChange={updates => updateRule(idx, updates)}
                  onRemove={() => removeRule(idx)}
                  onToggle={() => toggleExpand(idx)}
                />
              </FlexItem>
            ))}
          </Flex>
        )}
      </ModalBody>
      <ModalFooter>
        <Button onClick={handleSubmit} isLoading={submitting} isDisabled={(mode === 'create' && !name) || submitting}>
          {mode === 'create' ? 'Create' : 'Save'}
        </Button>
        <Button variant="link" onClick={onClose}>Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}

// --- Rule editor card ---

function RuleEditor({
  rule,
  index,
  allTargets,
  allEnvironments,
  allProviders,
  allTypes,
  onChange,
  onRemove,
  onToggle,
}: {
  rule: RuleDraft;
  index: number;
  allTargets: string[];
  allEnvironments: string[];
  allProviders: string[];
  allTypes: string[];
  onChange: (updates: Partial<RuleDraft>) => void;
  onRemove: () => void;
  onToggle: () => void;
}) {
  const [typeOpen, setTypeOpen] = useState(false);
  const [strategyOpen, setStrategyOpen] = useState(false);
  const [requiredOpen, setRequiredOpen] = useState(false);
  const [prefOpen, setPrefOpen] = useState(false);
  const [forbidOpen, setForbidOpen] = useState(false);

  const summary = [
    rule.name || `Rule ${index + 1}`,
    rule.matchType && `type=${rule.matchType}`,
    rule.required && `required=${rule.required}`,
    rule.preferred.length > 0 && `preferred=[${rule.preferred.join(',')}]`,
    rule.strategy && `strategy=${rule.strategy}`,
  ].filter(Boolean).join(' | ');

  return (
    <Card isExpanded={rule.expanded}>
      <CardTitle>
        <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
          <FlexItem
            style={{ cursor: 'pointer', flex: 1 }}
            onClick={onToggle}
          >
            <Flex gap={{ default: 'gapSm' }} alignItems={{ default: 'alignItemsCenter' }}>
              <FlexItem>
                <span style={{ fontSize: 14, color: '#6a6e73', marginRight: 4 }}>{rule.expanded ? '▼' : '▶'}</span>
                {summary}
              </FlexItem>
              {rule.priority > 0 && <FlexItem><Label isCompact color="blue">priority={rule.priority}</Label></FlexItem>}
            </Flex>
          </FlexItem>
          <FlexItem>
            <Button variant="plain" aria-label="Remove rule" onClick={onRemove}><TrashIcon /></Button>
          </FlexItem>
        </Flex>
      </CardTitle>
      {rule.expanded && (
        <CardBody>
          <Flex direction={{ default: 'column' }} gap={{ default: 'gapMd' }}>
            {/* Name & Priority */}
            <FlexItem>
              <Flex gap={{ default: 'gapMd' }}>
                <FlexItem grow={{ default: 'grow' }}>
                  <FormGroup label="Rule name" fieldId={`${rule.id}-name`}>
                    <TextInput id={`${rule.id}-name`} value={rule.name} onChange={(_e, v) => onChange({ name: v })} placeholder="prefer-cloud" />
                  </FormGroup>
                </FlexItem>
                <FlexItem style={{ width: 160 }}>
                  <FormGroup label="Priority" fieldId={`${rule.id}-priority`}>
                    <NumberInput
                      id={`${rule.id}-priority`}
                      value={rule.priority}
                      onChange={(e: React.FormEvent<HTMLInputElement>) => {
                        const v = parseInt((e.target as HTMLInputElement).value, 10);
                        if (!isNaN(v)) onChange({ priority: v });
                      }}
                      onMinus={() => onChange({ priority: Math.max(0, rule.priority - 10) })}
                      onPlus={() => onChange({ priority: rule.priority + 10 })}
                      min={0}
                    />
                  </FormGroup>
                </FlexItem>
              </Flex>
            </FlexItem>

            {/* Match section */}
            <FlexItem>
              <Content component="h4" style={{ marginBottom: 8 }}>Match conditions</Content>
              <Flex gap={{ default: 'gapMd' }} wrap={{ default: 'wrap' }}>
                <FlexItem grow={{ default: 'grow' }} style={{ minWidth: 200 }}>
                  <FormGroup label="Resource type" fieldId={`${rule.id}-type`}>
                    <Select
                      id={`${rule.id}-type`}
                      isOpen={typeOpen}
                      onOpenChange={setTypeOpen}
                      selected={rule.matchType || undefined}
                      onSelect={(_e, val) => { onChange({ matchType: val as string }); setTypeOpen(false); }}
                      toggle={(ref) => (
                        <MenuToggle ref={ref} onClick={() => setTypeOpen(!typeOpen)} style={{ width: '100%' }}>
                          {rule.matchType || 'Any type'}
                        </MenuToggle>
                      )}
                    >
                      <SelectList>
                        <SelectOption value="" key="any">Any type</SelectOption>
                        {allTypes.map(t => <SelectOption key={t} value={t}>{t}</SelectOption>)}
                      </SelectList>
                    </Select>
                  </FormGroup>
                </FlexItem>
                <FlexItem grow={{ default: 'grow' }} style={{ minWidth: 250 }}>
                  <FormGroup label="Labels (JSON)" fieldId={`${rule.id}-labels`}>
                    <TextInput
                      id={`${rule.id}-labels`}
                      value={rule.matchLabels}
                      onChange={(_e, v) => onChange({ matchLabels: v })}
                      placeholder='{"env": "production"}'
                    />
                  </FormGroup>
                </FlexItem>
              </Flex>
              <FormGroup label="CEL expression" fieldId={`${rule.id}-cel`} style={{ marginTop: 8 }}>
                <TextInput
                  id={`${rule.id}-cel`}
                  value={rule.matchExpression}
                  onChange={(_e, v) => onChange({ matchExpression: v })}
                  placeholder='component.type == "container" && has(component.labels.tier)'
                  style={{ fontFamily: 'monospace', fontSize: 13 }}
                />
              </FormGroup>
            </FlexItem>

            {/* Provider / Environment selection */}
            <FlexItem>
              <Content component="h4" style={{ marginBottom: 8 }}>Provider &amp; environment selection</Content>
              <Flex gap={{ default: 'gapMd' }} wrap={{ default: 'wrap' }}>
                <FlexItem grow={{ default: 'grow' }} style={{ minWidth: 200 }}>
                  <FormGroup label="Required" fieldId={`${rule.id}-required`}>
                    <Select
                      id={`${rule.id}-required`}
                      isOpen={requiredOpen}
                      onOpenChange={setRequiredOpen}
                      selected={rule.required || undefined}
                      onSelect={(_e, val) => { onChange({ required: val as string }); setRequiredOpen(false); }}
                      toggle={(ref) => (
                        <MenuToggle ref={ref} onClick={() => setRequiredOpen(!requiredOpen)} style={{ width: '100%' }}>
                          {rule.required || 'None'}
                        </MenuToggle>
                      )}
                    >
                      <SelectList>
                        <SelectOption value="" key="none">None</SelectOption>
                        {allEnvironments.length > 0 && allEnvironments.map(e => (
                          <SelectOption key={`env-${e}`} value={e} description="environment">{e}</SelectOption>
                        ))}
                        {allProviders.map(p => (
                          <SelectOption key={`prov-${p}`} value={p} description="provider type">{p}</SelectOption>
                        ))}
                      </SelectList>
                    </Select>
                  </FormGroup>
                </FlexItem>
                <FlexItem grow={{ default: 'grow' }} style={{ minWidth: 200 }}>
                  <FormGroup label="Strategy" fieldId={`${rule.id}-strategy`}>
                    <Select
                      id={`${rule.id}-strategy`}
                      isOpen={strategyOpen}
                      onOpenChange={setStrategyOpen}
                      selected={rule.strategy || undefined}
                      onSelect={(_e, val) => { onChange({ strategy: val as string }); setStrategyOpen(false); }}
                      toggle={(ref) => (
                        <MenuToggle ref={ref} onClick={() => setStrategyOpen(!strategyOpen)} style={{ width: '100%' }}>
                          {rule.strategy || 'Default (first)'}
                        </MenuToggle>
                      )}
                    >
                      <SelectList>
                        <SelectOption value="" key="default">Default (first)</SelectOption>
                        {STRATEGIES.map(s => <SelectOption key={s} value={s}>{s}</SelectOption>)}
                      </SelectList>
                    </Select>
                  </FormGroup>
                </FlexItem>
              </Flex>

              <Flex gap={{ default: 'gapMd' }} style={{ marginTop: 8 }} wrap={{ default: 'wrap' }}>
                <FlexItem grow={{ default: 'grow' }} style={{ minWidth: 200 }}>
                  <FormGroup label="Preferred" fieldId={`${rule.id}-preferred`}>
                    <Select
                      id={`${rule.id}-preferred`}
                      isOpen={prefOpen}
                      onOpenChange={setPrefOpen}
                      selected={rule.preferred}
                      onSelect={(_e, val) => {
                        const v = val as string;
                        const next = rule.preferred.includes(v) ? rule.preferred.filter(p => p !== v) : [...rule.preferred, v];
                        onChange({ preferred: next });
                      }}
                      toggle={(ref) => (
                        <MenuToggle ref={ref} onClick={() => setPrefOpen(!prefOpen)} style={{ width: '100%' }}>
                          {rule.preferred.length > 0 ? rule.preferred.join(', ') : 'None'}
                        </MenuToggle>
                      )}
                    >
                      <SelectList>
                        {allTargets.map(t => (
                          <SelectOption key={t} value={t} hasCheckbox isSelected={rule.preferred.includes(t)}
                            description={allEnvironments.includes(t) ? 'environment' : 'provider type'}
                          >{t}</SelectOption>
                        ))}
                      </SelectList>
                    </Select>
                  </FormGroup>
                </FlexItem>
                <FlexItem grow={{ default: 'grow' }} style={{ minWidth: 200 }}>
                  <FormGroup label="Forbidden" fieldId={`${rule.id}-forbidden`}>
                    <Select
                      id={`${rule.id}-forbidden`}
                      isOpen={forbidOpen}
                      onOpenChange={setForbidOpen}
                      selected={rule.forbidden}
                      onSelect={(_e, val) => {
                        const v = val as string;
                        const next = rule.forbidden.includes(v) ? rule.forbidden.filter(p => p !== v) : [...rule.forbidden, v];
                        onChange({ forbidden: next });
                      }}
                      toggle={(ref) => (
                        <MenuToggle ref={ref} onClick={() => setForbidOpen(!forbidOpen)} style={{ width: '100%' }}>
                          {rule.forbidden.length > 0 ? rule.forbidden.join(', ') : 'None'}
                        </MenuToggle>
                      )}
                    >
                      <SelectList>
                        {allTargets.map(t => (
                          <SelectOption key={t} value={t} hasCheckbox isSelected={rule.forbidden.includes(t)}
                            description={allEnvironments.includes(t) ? 'environment' : 'provider type'}
                          >{t}</SelectOption>
                        ))}
                      </SelectList>
                    </Select>
                  </FormGroup>
                </FlexItem>
              </Flex>
            </FlexItem>

            {/* Injected properties */}
            <FlexItem>
              <FormGroup label="Injected properties (JSON, optional)" fieldId={`${rule.id}-props`}>
                <TextArea
                  id={`${rule.id}-props`}
                  value={rule.properties}
                  onChange={(_e, v) => onChange({ properties: v })}
                  rows={3}
                  placeholder='{"replicas": 3, "cpu": "500m"}'
                  style={{ fontFamily: 'monospace', fontSize: 13 }}
                />
              </FormGroup>
            </FlexItem>
          </Flex>
        </CardBody>
      )}
    </Card>
  );
}
