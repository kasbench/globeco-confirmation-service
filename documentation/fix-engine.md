
<a name="public.execution"></a>
### _public_.**execution** `Table`
| Name | Data type  | PK | FK | UQ  | Not null | Default value | Description |
| --- | --- | :---: | :---: | :---: | :---: | --- | --- |
| id | serial | &#10003; |  |  | &#10003; |  |  |
| execution_service_id | integer |  |  |  | &#10003; |  |  |
| is_open | boolean |  |  |  | &#10003; | true |  |
| execution_status | varchar(20) |  |  |  | &#10003; |  |  |
| trade_type | varchar(10) |  |  |  | &#10003; |  |  |
| destination | varchar(20) |  |  |  | &#10003; |  |  |
| security_id | char(24) |  |  |  | &#10003; |  |  |
| ticker | varchar(20) |  |  |  | &#10003; |  |  |
| quantity_ordered | decimal(18,8) |  |  |  | &#10003; |  |  |
| limit_price | decimal(18,8) |  |  |  |  |  |  |
| received_timestamp | timestamptz |  |  |  | &#10003; |  |  |
| sent_timestamp | timestamptz |  |  |  | &#10003; |  |  |
| last_fill_timestamp | timestamptz |  |  |  |  |  |  |
| quantity_filled | decimal(18,8) |  |  |  | &#10003; | 0 |  |
| next_fill_timestamp | timestamptz |  |  |  |  |  |  |
| number_of_fills | smallint |  |  |  | &#10003; | 0 |  |
| total_amount | decimal(18,8) |  |  |  | &#10003; | 0 |  |
| trade_service_execution_id | integer |  |  |  |  |  |  |
| version | integer |  |  |  | &#10003; | 1 |  |

#### Constraints
| Name | Type | Column(s) | References | On Update | On Delete | Expression | Description |
|  --- | --- | --- | --- | --- | --- | --- | --- |
| execution_pk | PRIMARY KEY | id |  |  |  |  |  |

#### Indexes
| Name | Type | Column(s) | Expression(s) | Predicate | Description |
|  --- | --- | --- | --- | --- | --- |
| execution_order_id_ndx | btree | execution_service_id |  |  |  |
| execution_next_fill_ndx | btree | next_fill_timestamp |  |  |  |

---

Generated at _2025-05-24T15:41:22_ by **pgModeler 1.2.0-beta1**
[PostgreSQL Database Modeler - pgmodeler.io ](https://pgmodeler.io)
Copyright © 2006 - 2025 Raphael Araújo e Silva 
