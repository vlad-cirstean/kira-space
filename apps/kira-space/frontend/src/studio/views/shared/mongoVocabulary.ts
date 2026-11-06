// The operator table has a second consumer — the console's own Mongo completion source — rather
// than a second copy that could drift from this one. find()'s filter document is the only thing
// either surface reaches — aggregation-stage operators ($group, $lookup, …) have no business in
// either.
export const MONGO_QUERY_OPERATORS: readonly string[] = [
  '$eq',
  '$ne',
  '$gt',
  '$gte',
  '$lt',
  '$lte',
  '$in',
  '$nin',
  '$exists',
  '$type',
  '$regex',
  '$options',
  '$and',
  '$or',
  '$nor',
  '$not',
  '$all',
  '$elemMatch',
  '$size',
];

// The six BSON constructors engine/adapters/mongo/literal.ts's CONSTRUCTORS can build,
// surfaced in the filter bar and the Mongo console alike — the capability already existed
// (`resolveEjsonWrappers`/`parseDocumentLiteral`) and nothing surfaced it, which reads
// the same as it not existing. Each inserts a full call with an empty argument, caret positioned
// inside it (`caretOffsetFromEnd`) rather than after the closing paren.
export interface MongoValueConstructor {
  name: string;
  insert: string;
  caretOffsetFromEnd: number;
}

export const MONGO_VALUE_CONSTRUCTORS: readonly MongoValueConstructor[] = [
  { name: 'ObjectId', insert: "ObjectId('')", caretOffsetFromEnd: 2 },
  { name: 'ISODate', insert: "ISODate('')", caretOffsetFromEnd: 2 },
  { name: 'Date', insert: "Date('')", caretOffsetFromEnd: 2 },
  { name: 'NumberLong', insert: "NumberLong('')", caretOffsetFromEnd: 2 },
  { name: 'NumberInt', insert: 'NumberInt()', caretOffsetFromEnd: 1 },
  { name: 'NumberDecimal', insert: "NumberDecimal('')", caretOffsetFromEnd: 2 },
];
