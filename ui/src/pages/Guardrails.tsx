import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  Card,
  CardBody,
  CardTitle,
  ClipboardCopy,
  ClipboardCopyVariant,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  EmptyState,
  EmptyStateBody,
  Flex,
  FlexItem,
  FormGroup,
  Label,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  PageSection,
  Select,
  SelectList,
  SelectOption,
  Spinner,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { CheckCircleIcon, ExclamationCircleIcon } from '@patternfly/react-icons';
import {
  compliance,
  applications as applicationsApi,
  type ApplicationRecord,
  type ComplianceCheckResult,
  type ComplianceViolation,
} from '../api/client';

const SAMPLE_POLICY = `package dcm.compliance

# Production databases must have at least 10Gi storage.
deny contains msg if {
    input.component.type == "postgres"
    input.environment.labels.env == "prod"
    storage := input.component.properties.storage
    not startswith(storage, "10")
    not startswith(storage, "20")
    not startswith(storage, "50")
    not startswith(storage, "100")
    msg := sprintf("postgres %q in prod requires at least 10Gi storage, got %s",
                   [input.component.name, storage])
}

# Containers in production must specify replicas.
deny contains msg if {
    input.component.type == "container"
    input.environment.labels.env == "prod"
    not input.component.properties.replicas
    msg := sprintf("container %q in prod must specify replicas",
                   [input.component.name])
}

# No environment should exceed $1/hr.
deny contains msg if {
    input.environment.cost.hourlyRate > 1.0
    msg := sprintf("environment %q exceeds cost limit ($%.2f/hr)",
                   [input.environment.name, input.environment.cost.hourlyRate])
}`;

export default function Guardrails() {
  const [apps, setApps] = useState<ApplicationRecord[]>([]);
  const [selectedApp, setSelectedApp] = useState('');
  const [appSelectOpen, setAppSelectOpen] = useState(false);
  const [checking, setChecking] = useState(false);
  const [result, setResult] = useState<ComplianceCheckResult | null>(null);
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [error, setError] = useState('');
  const [showSample, setShowSample] = useState(false);

  useEffect(() => {
    applicationsApi.list().then(setApps).catch(e => setError(e.message));
  }, []);

  const runCheck = useCallback(async () => {
    if (!selectedApp) return;
    setChecking(true);
    setError('');
    setResult(null);
    try {
      const res = await compliance.check(selectedApp);
      setResult(res);
      setHistory(prev => [{
        application: selectedApp,
        passed: res.passed,
        violations: res.violations,
        timestamp: new Date().toLocaleString(),
      }, ...prev].slice(0, 20));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setChecking(false);
    }
  }, [selectedApp]);

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Guardrails</Content>
        <Content component="p" style={{ color: 'var(--pf-t--global--text--color--subtle)', marginTop: 4 }}>
          OPA/Rego rules that block deployments violating organizational standards.
          Guardrails are evaluated after planning but before any resources are created.
        </Content>
      </PageSection>
      <PageSection>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}

        {/* Compliance check */}
        <Card style={{ marginBottom: 24 }}>
          <CardTitle>Check Compliance</CardTitle>
          <CardBody>
            <Content component="p" style={{ marginBottom: 16, color: 'var(--pf-t--global--text--color--subtle)' }}>
              Run all loaded guardrails against an application to preview whether a deployment would be allowed.
            </Content>
            <Flex gap={{ default: 'gapMd' }} alignItems={{ default: 'alignItemsFlexEnd' }}>
              <FlexItem grow={{ default: 'grow' }} style={{ maxWidth: 400 }}>
                <FormGroup label="Application" fieldId="compliance-app">
                  <Select
                    id="compliance-app"
                    isOpen={appSelectOpen}
                    onOpenChange={setAppSelectOpen}
                    selected={selectedApp || undefined}
                    onSelect={(_e, val) => { setSelectedApp(val as string); setAppSelectOpen(false); setResult(null); }}
                    toggle={(ref) => (
                      <MenuToggle ref={ref} onClick={() => setAppSelectOpen(!appSelectOpen)} style={{ width: '100%' }}>
                        {selectedApp || 'Select an application'}
                      </MenuToggle>
                    )}
                  >
                    <SelectList>
                      {apps.map(a => <SelectOption key={a.name} value={a.name}>{a.name}</SelectOption>)}
                    </SelectList>
                  </Select>
                </FormGroup>
              </FlexItem>
              <FlexItem>
                <Button
                  onClick={runCheck}
                  isDisabled={!selectedApp || checking}
                  isLoading={checking}
                >
                  Run check
                </Button>
              </FlexItem>
            </Flex>

            {result && (
              <div style={{ marginTop: 24 }}>
                {result.message ? (
                  <Alert variant="info" title={result.message} isInline />
                ) : result.passed ? (
                  <Alert
                    variant="success"
                    title={`All guardrails passed for "${result.application}"`}
                    isInline
                    customIcon={<CheckCircleIcon />}
                  />
                ) : (
                  <>
                    <Alert
                      variant="danger"
                      title={`${result.violations.length} violation(s) found for "${result.application}"`}
                      isInline
                      customIcon={<ExclamationCircleIcon />}
                      style={{ marginBottom: 16 }}
                    />
                    <Table aria-label="Violations" variant="compact">
                      <Thead>
                        <Tr>
                          <Th width={5}>#</Th>
                          <Th>Violation</Th>
                        </Tr>
                      </Thead>
                      <Tbody>
                        {result.violations.map((v, i) => (
                          <Tr key={i}>
                            <Td dataLabel="#">{i + 1}</Td>
                            <Td dataLabel="Violation">
                              <code style={{ fontSize: 13 }}>{v.message}</code>
                            </Td>
                          </Tr>
                        ))}
                      </Tbody>
                    </Table>
                  </>
                )}
              </div>
            )}
          </CardBody>
        </Card>

        {/* How it works */}
        <Card style={{ marginBottom: 24 }}>
          <CardTitle>How Guardrails Work</CardTitle>
          <CardBody>
            <DescriptionList isHorizontal>
              <DescriptionListGroup>
                <DescriptionListTerm>Language</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label isCompact color="blue">OPA / Rego v1</Label>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>When evaluated</DescriptionListTerm>
                <DescriptionListDescription>After planning, before applying resources</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Effect</DescriptionListTerm>
                <DescriptionListDescription>Violations block the deployment entirely</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Policy location</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>data/policies/*.rego</code> — loaded on server startup
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Package</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>package dcm.compliance</code>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Rule format</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>{'deny contains msg if { ... }'}</code>
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
            <Toolbar style={{ padding: 0, marginTop: 16 }}>
              <ToolbarContent>
                <ToolbarItem>
                  <Button variant="secondary" onClick={() => setShowSample(true)}>View sample policy</Button>
                </ToolbarItem>
              </ToolbarContent>
            </Toolbar>
          </CardBody>
        </Card>

        {/* Check history */}
        {history.length > 0 && (
          <Card>
            <CardTitle>Recent Checks</CardTitle>
            <CardBody>
              <Table aria-label="Check history" variant="compact">
                <Thead>
                  <Tr>
                    <Th>Application</Th>
                    <Th>Result</Th>
                    <Th>Violations</Th>
                    <Th>Time</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {history.map((h, i) => (
                    <Tr key={i}>
                      <Td dataLabel="Application">{h.application}</Td>
                      <Td dataLabel="Result">
                        {h.passed ? (
                          <Label color="green" isCompact icon={<CheckCircleIcon />}>Passed</Label>
                        ) : (
                          <Label color="red" isCompact icon={<ExclamationCircleIcon />}>Failed</Label>
                        )}
                      </Td>
                      <Td dataLabel="Violations">{h.violations.length}</Td>
                      <Td dataLabel="Time">{h.timestamp}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </CardBody>
          </Card>
        )}
      </PageSection>

      {/* Sample policy modal */}
      <Modal isOpen={showSample} onClose={() => setShowSample(false)} variant="large">
        <ModalHeader title="Sample Guardrail Policy" />
        <ModalBody>
          <Content component="p" style={{ marginBottom: 16, color: 'var(--pf-t--global--text--color--subtle)' }}>
            Save this as a <code>.rego</code> file in <code>data/policies/</code> and restart the server to activate it.
          </Content>
          <ClipboardCopy
            isCode
            isReadOnly
            variant={ClipboardCopyVariant.expansion}
            hoverTip="Copy"
            clickTip="Copied"
            style={{ fontFamily: 'monospace', fontSize: 13, whiteSpace: 'pre' }}
          >
            {SAMPLE_POLICY}
          </ClipboardCopy>
        </ModalBody>
        <ModalFooter>
          <Button variant="link" onClick={() => setShowSample(false)}>Close</Button>
        </ModalFooter>
      </Modal>
    </>
  );
}

interface HistoryEntry {
  application: string;
  passed: boolean;
  violations: ComplianceViolation[];
  timestamp: string;
}
