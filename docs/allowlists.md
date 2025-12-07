# Access Control with Allowlists

## Overview

The MCP Trino server supports hierarchical allowlist filtering to restrict access to specific catalogs, schemas, and tables. This feature provides **performance optimization** and **additional access control** on top of your existing Trino security configuration.

## Key Benefits

- **🚀 Performance**: Dramatically reduces AI assistant query time by limiting search scope
- **🎯 Focus**: Eliminates distractions from irrelevant data sources
- **🔒 Security**: Additional layer of access control (complements Trino's built-in security)
- **🎛️ Flexibility**: Independent filtering at catalog, schema, and table levels

## Configuration

Configure allowlists using environment variables with comma-separated values:

### Environment Variables

| Variable | Description | Format | Example |
|----------|-------------|---------|---------|
| `TRINO_ALLOWED_CATALOGS` | Restrict visible catalogs | `catalog1,catalog2` | `hive,postgresql` |
| `TRINO_ALLOWED_SCHEMAS` | Restrict visible schemas | `catalog.schema` | `hive.analytics,hive.marts` |
| `TRINO_ALLOWED_TABLES` | Restrict visible tables | `catalog.schema.table` | `hive.analytics.users` |

### Format Requirements

- **Schemas**: Must include catalog name (e.g., `hive.analytics`)
- **Tables**: Must include catalog and schema (e.g., `hive.analytics.users`)
- **Case insensitive**: `HIVE.Analytics` matches `hive.analytics`
- **Whitespace tolerant**: Spaces around commas are automatically trimmed
- **Empty values**: Empty allowlists mean no filtering (all items accessible)

## Usage Examples

### Common Use Cases

#### 1. Focus AI on Specific Schemas (Most Common)

*Problem: Claude AI searches through 20+ schemas, causing performance issues*

```bash
# Solution: Limit to only the schemas you need
export TRINO_ALLOWED_SCHEMAS="hive.analytics,hive.marts,hive.reporting"
```

**Result**: AI assistant only sees 3 schemas instead of 20+, dramatically improving performance.

#### 2. Multi-Catalog Environment

```bash
# Allow specific catalogs and their schemas
export TRINO_ALLOWED_CATALOGS="hive,postgresql"
export TRINO_ALLOWED_SCHEMAS="hive.analytics,hive.marts,postgresql.public"
```

#### 3. Production Security Layer

```bash
# Fine-grained control: specific catalogs, schemas, and sensitive tables
export TRINO_ALLOWED_CATALOGS="production_hive,reporting_db"
export TRINO_ALLOWED_SCHEMAS="production_hive.clean_data,reporting_db.dashboards"
export TRINO_ALLOWED_TABLES="production_hive.clean_data.customer_summary"
```

#### 4. Development Environment

```bash
# Allow everything in development (default behavior)
# Don't set any allowlist environment variables
```

## How It Works

### Hierarchical Independence

Each allowlist level operates independently:

```bash
export TRINO_ALLOWED_SCHEMAS="hive.analytics,hive.marts"
export TRINO_ALLOWED_TABLES="hive.analytics.users"
```

- `list_schemas` returns: `analytics, marts` (from schema allowlist)
- `list_tables` in `hive.analytics` returns: `users` (from table allowlist)
- `list_tables` in `hive.marts` returns: all tables (no table restriction for this schema)

### Parameter Resolution

The server handles flexible table references:

```bash
export TRINO_ALLOWED_TABLES="hive.analytics.users"
```

All these calls work correctly:

- `get_table_schema("hive", "analytics", "users")` ✅
- `get_table_schema("", "analytics", "users")` ✅ (uses default catalog)
- `get_table_schema("", "", "analytics.users")` ✅ (schema.table format)
- `get_table_schema("", "", "hive.analytics.users")` ✅ (fully qualified)

## Error Handling

### Configuration Errors

The server validates allowlist formats on startup:

```bash
# ❌ Wrong format for schemas (missing catalog)
export TRINO_ALLOWED_SCHEMAS="analytics,marts"
# Error: invalid format in TRINO_ALLOWED_SCHEMAS: 'analytics' (expected 1 dots, found 0)

# ✅ Correct format
export TRINO_ALLOWED_SCHEMAS="hive.analytics,hive.marts"
```

### Access Denied Errors

When access is restricted:

```bash
# With allowlist: TRINO_ALLOWED_TABLES="hive.analytics.users"
get_table_schema("hive", "analytics", "orders")
# Error: table access denied: hive.analytics.orders not in allowlist
```

## Performance Impact

### Before Allowlists

```
AI Query: "Show me sales data"
↓
Scans: 25 catalogs × 50 schemas = 1,250 metadata queries
↓
Time: 30-60 seconds
```

### After Allowlists

```bash
export TRINO_ALLOWED_SCHEMAS="hive.sales,hive.analytics,hive.marts"
```

```
AI Query: "Show me sales data"
↓
Scans: Only 3 schemas = 3 metadata queries
↓
Time: 2-5 seconds
```

**Result: 10-20x performance improvement for AI queries**

## Security Considerations

### Complementary Security

Allowlists are **additional** access control, not replacements:

- ✅ **Use with Trino security**: LDAP, Kerberos, role-based access
- ✅ **Defense in depth**: Multiple security layers
- ✅ **Fail-safe**: Restricted allowlists are more secure than open access

### Important Notes

- **Not primary security**: Don't rely solely on allowlists for sensitive data
- **Bypass possible**: Users with direct Trino access can still access restricted data
- **Audit compliance**: Allowlists help with data governance and audit requirements

## Troubleshooting

### Common Issues

#### 1. "No catalogs/schemas/tables returned"

```bash
# Check if allowlist is too restrictive
echo $TRINO_ALLOWED_CATALOGS
# Temporarily disable to test
unset TRINO_ALLOWED_CATALOGS
```

#### 2. "Table access denied" errors

```bash
# Verify table format includes catalog and schema
export TRINO_ALLOWED_TABLES="hive.analytics.users"  # ✅ Correct
export TRINO_ALLOWED_TABLES="users"                 # ❌ Wrong format
```

#### 3. Case sensitivity issues

```bash
# All these are equivalent (case-insensitive matching):
export TRINO_ALLOWED_SCHEMAS="HIVE.ANALYTICS"
export TRINO_ALLOWED_SCHEMAS="hive.analytics"
export TRINO_ALLOWED_SCHEMAS="Hive.Analytics"
```

### Debug Mode

Enable debug logging to see filtering in action:

```bash
# Server logs will show:
# DEBUG: Catalog filtering: 10 catalogs -> 2 catalogs
# DEBUG: Schema filtering: 25 schemas -> 3 schemas
# DEBUG: Table filtering: 100 tables -> 5 tables
```

## Migration Guide

### From No Allowlists to Allowlists

1. **Identify current usage**: Check which schemas AI assistants actually use
2. **Start conservative**: Begin with schema-level filtering
3. **Monitor performance**: Measure query time improvements
4. **Refine gradually**: Add table-level restrictions if needed

```bash
# Step 1: Identify active schemas by monitoring Trino query logs
# Step 2: Configure schema allowlist
export TRINO_ALLOWED_SCHEMAS="most_used_schema1,most_used_schema2"
# Step 3: Test AI assistant performance
# Step 4: Add more restrictions if needed
```

### Rollback Strategy

To disable allowlists completely:

```bash
unset TRINO_ALLOWED_CATALOGS
unset TRINO_ALLOWED_SCHEMAS
unset TRINO_ALLOWED_TABLES
# Restart mcp-trino server
```

## Best Practices

### Performance Optimization

1. **Start with schema filtering**: Biggest performance impact
2. **Use specific catalogs**: Avoid scanning unused data sources
3. **Monitor query patterns**: Adjust allowlists based on actual usage

### Security Best Practices

1. **Principle of least privilege**: Only allow necessary access
2. **Regular reviews**: Audit and update allowlists quarterly
3. **Document decisions**: Maintain clear justification for allowlist choices
4. **Test changes**: Validate allowlist updates in non-production first

### Operational Guidelines

1. **Environment-specific**: Different allowlists for dev/staging/production
2. **Version control**: Store allowlist configurations in infrastructure as code
3. **Monitoring**: Track allowlist effectiveness and performance improvements
4. **Documentation**: Keep team informed about access restrictions

## Related Documentation

- [Deployment Guide](deployment.md) - Full server configuration options
- [Tools Reference](tools.md) - MCP tool descriptions and usage
- [Integration Guide](integrations.md) - Client setup with allowlists
