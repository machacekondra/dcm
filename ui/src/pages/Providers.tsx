import { useEffect, useState } from 'react';
import {
  Alert,
  Card,
  CardBody,
  CardTitle,
  Content,
  Gallery,
  Label,
  LabelGroup,
  PageSection,
  Spinner,
} from '@patternfly/react-core';
import { providers as providersApi, type ProviderInfo } from '../api/client';

export default function Providers() {
  const [list, setList] = useState<ProviderInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    providersApi.list()
      .then(setList)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  if (error) return <PageSection><Alert variant="danger" title={error} /></PageSection>;
  if (loading) return <PageSection><Spinner /></PageSection>;

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Providers</Content>
        <Content component="p">
          {list.length} provider(s) registered
        </Content>
      </PageSection>
      <PageSection>
      <Gallery hasGutter minWidths={{ default: '300px' }}>
        {list.map(p => (
          <Card key={p.name}>
            <CardTitle>{p.name}</CardTitle>
            <CardBody>
              <Content component="small" style={{ display: 'block', marginBottom: 8 }}>Capabilities</Content>
              <LabelGroup>
                {p.capabilities.map(cap => (
                  <Label key={cap} color="blue">{cap}</Label>
                ))}
              </LabelGroup>
            </CardBody>
          </Card>
        ))}
      </Gallery>
    </PageSection>
    </>
  );
}
