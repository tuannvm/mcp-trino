# MCP Tools Reference

The server provides the following MCP tools for interacting with Trino:

## execute_query

Execute a SQL query against Trino with full SQL support for complex analytical queries.

**Sample Prompt:**
> "How many customers do we have per region? Can you show them in descending order?"

**Example:**
```json
{
  "query": "SELECT region, COUNT(*) as customer_count FROM tpch.tiny.customer GROUP BY region ORDER BY customer_count DESC"
}
```

**Response:**
```json
{
  "columns": ["region", "customer_count"],
  "data": [
    ["AFRICA", 5],
    ["AMERICA", 5],
    ["ASIA", 5],
    ["EUROPE", 5],
    ["MIDDLE EAST", 5]
  ]
}
```

## list_catalogs

List all catalogs available in the Trino server, providing a comprehensive view of your data ecosystem.

**Sample Prompt:**
> "What databases do we have access to in our Trino environment?"

**Example:**
```json
{}
```

**Response:**
```json
{
  "catalogs": ["tpch", "memory", "system", "jmx"]
}
```

## list_schemas

List all schemas in a catalog, helping you navigate through the data hierarchy efficiently.

**Sample Prompt:**
> "What schemas or datasets are available in the tpch catalog?"

**Example:**
```json
{
  "catalog": "tpch"
}
```

**Response:**
```json
{
  "schemas": ["information_schema", "sf1", "sf100", "sf1000", "tiny"]
}
```

## list_tables

List all tables in a schema, giving you visibility into available datasets.

**Sample Prompt:**
> "What tables are available in the tpch tiny schema? I need to know what data we can query."

**Example:**
```json
{
  "catalog": "tpch",
  "schema": "tiny"
}
```

**Response:**
```json
{
  "tables": ["customer", "lineitem", "nation", "orders", "part", "partsupp", "region", "supplier"]
}
```

## get_table_schema

Get the schema of a table, understanding the structure of your data for better query planning.

**Sample Prompt:**
> "What columns are in the customer table? I need to know the data types and structure before writing my query."

**Example:**
```json
{
  "catalog": "tpch",
  "schema": "tiny",
  "table": "customer"
}
```

**Response:**
```json
{
  "columns": [
    {
      "name": "custkey",
      "type": "bigint",
      "nullable": false
    },
    {
      "name": "name",
      "type": "varchar",
      "nullable": false
    },
    {
      "name": "address",
      "type": "varchar",
      "nullable": false
    },
    {
      "name": "nationkey",
      "type": "bigint",
      "nullable": false
    },
    {
      "name": "phone",
      "type": "varchar",
      "nullable": false
    },
    {
      "name": "acctbal",
      "type": "double",
      "nullable": false
    },
    {
      "name": "mktsegment",
      "type": "varchar",
      "nullable": false
    },
    {
      "name": "comment",
      "type": "varchar",
      "nullable": false
    }
  ]
}
```

## explain_query

Analyze Trino query execution plans without running expensive queries, showing distributed execution stages and resource estimates.

**Sample Prompt:**
> "Can you explain how this query will be executed? I want to understand the performance characteristics before running it on production data."

**Example:**
```json
{
  "query": "SELECT region, COUNT(*) as customer_count FROM tpch.tiny.customer GROUP BY region ORDER BY customer_count DESC",
  "format": "LOGICAL"
}
```

**Response:**
```json
{
  "execution_plan": [
    {
      "stage": "Fragment 0 [SINGLE]",
      "operations": [
        "Output[region, customer_count]",
        "Sort[customer_count DESC NULLS LAST]",
        "RemoteSource[sourceFragmentIds=[1]]"
      ]
    },
    {
      "stage": "Fragment 1 [HASH]",
      "operations": [
        "Aggregate[region, COUNT(*)]",
        "TableScan[tpch:customer]"
      ],
      "estimated_rows": 25,
      "estimated_cost": "cpu: 0.25, memory: 0.00, network: 0.05"
    }
  ]
}
```

This information is invaluable for understanding the column names, data types, and nullability constraints before writing queries against the table.

## End-to-End Example

Here's a complete interaction example showing how an AI assistant might use these tools to answer a business question:

**User Query:** "Can you help me analyze our biggest customers? I want to know the top 5 customers with the highest account balances."

**AI Assistant's workflow:**
1. First, discover available catalogs
   ```
   > Using list_catalogs tool
   > Discovers tpch catalog
   ```

2. Then, find available schemas
   ```
   > Using list_schemas tool with catalog "tpch"
   > Discovers "tiny" schema
   ```

3. Explore available tables
   ```
   > Using list_tables tool with catalog "tpch" and schema "tiny"
   > Finds "customer" table
   ```

4. Check the customer table schema
   ```
   > Using get_table_schema tool
   > Discovers "custkey", "name", "acctbal" and other columns
   ```

5. Finally, execute the query
   ```
   > Using execute_query tool with:
   > "SELECT custkey, name, acctbal FROM tpch.tiny.customer ORDER BY acctbal DESC LIMIT 5"
   ```

6. Returns the results to the user:
   ```
   The top 5 customers with highest account balances are:
   1. Customer #65 (Customer#000000065): $9,222.78
   2. Customer #13 (Customer#000000013): $8,270.47
   3. Customer #89 (Customer#000000089): $7,990.56
   4. Customer #11 (Customer#000000011): $7,912.91
   5. Customer #82 (Customer#000000082): $7,629.41
   ```

This seamless workflow demonstrates how the MCP tools enable AI assistants to explore and query data in a conversational manner.