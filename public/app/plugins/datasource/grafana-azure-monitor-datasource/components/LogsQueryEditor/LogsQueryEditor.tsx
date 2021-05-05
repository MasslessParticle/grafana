import React from 'react';
import { AzureMonitorErrorish, AzureMonitorOption, AzureMonitorQuery } from '../../types';
import Datasource from '../../datasource';
import { InlineFieldRow } from '@grafana/ui';
import SubscriptionField from '../SubscriptionField';
import WorkspaceField from './WorkspaceField';
import QueryField from './QueryField';
import FormatAsField from './FormatAsField';
import ResourceField from './ResourceField';

interface LogsQueryEditorProps {
  query: AzureMonitorQuery;
  datasource: Datasource;
  subscriptionId: string;
  onChange: (newQuery: AzureMonitorQuery) => void;
  variableOptionGroup: { label: string; options: AzureMonitorOption[] };
  setError: (source: string, error: AzureMonitorErrorish | undefined) => void;
}

const SHOW_RESOURCE_FIELD = true;

const LogsQueryEditor: React.FC<LogsQueryEditorProps> = ({
  query,
  datasource,
  subscriptionId,
  variableOptionGroup,
  onChange,
  setError,
}) => {
  return (
    <div data-testid="azure-monitor-logs-query-editor">
      {SHOW_RESOURCE_FIELD ? (
        <InlineFieldRow>
          <ResourceField
            query={query}
            datasource={datasource}
            subscriptionId={subscriptionId}
            variableOptionGroup={variableOptionGroup}
            onQueryChange={onChange}
            setError={setError}
          />
        </InlineFieldRow>
      ) : (
        <InlineFieldRow>
          <SubscriptionField
            query={query}
            datasource={datasource}
            subscriptionId={subscriptionId}
            variableOptionGroup={variableOptionGroup}
            onQueryChange={onChange}
            setError={setError}
          />
          <WorkspaceField
            query={query}
            datasource={datasource}
            subscriptionId={subscriptionId}
            variableOptionGroup={variableOptionGroup}
            onQueryChange={onChange}
            setError={setError}
          />
        </InlineFieldRow>
      )}

      <QueryField
        query={query}
        datasource={datasource}
        subscriptionId={subscriptionId}
        variableOptionGroup={variableOptionGroup}
        onQueryChange={onChange}
        setError={setError}
      />

      <FormatAsField
        query={query}
        datasource={datasource}
        subscriptionId={subscriptionId}
        variableOptionGroup={variableOptionGroup}
        onQueryChange={onChange}
        setError={setError}
      />
    </div>
  );
};

export default LogsQueryEditor;
