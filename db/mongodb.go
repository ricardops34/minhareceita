package db

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"codeberg.org/cuducos/minha-receita/transform"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoRecord struct {
	Id   string            `json:"id" bson:"id"`
	Json transform.Company `json:"json" bson:"json"`
}

type MongoDB struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongoDB initializes a new MongoDB connection wrapped in a structure.
func NewMongoDB(uri string) (MongoDB, error) {
	opts := options.Client().ApplyURI(uri)
	c, err := mongo.Connect(opts)
	if err != nil {
		return MongoDB{}, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx, nil); err != nil {
		return MongoDB{}, fmt.Errorf("failed to ping to MongoDB: %w", err)
	}
	u := strings.Split(uri, "?")[0] // Remove query parameters from the URI
	ps := strings.Split(u, "/")
	n := ps[len(ps)-1]
	if n == "" || strings.Contains(n, "@") { // ensure the database name is valid
		return MongoDB{}, fmt.Errorf("no database name found in the uri")
	}
	return MongoDB{client: c, db: c.Database(n)}, nil
}

// Create creates the required collections.
func (m *MongoDB) Create() error {
	for _, c := range []string{companyTableName, metaTableName} {
		slog.Info("Creating", "collection", c)
		if err := m.db.CreateCollection(context.Background(), c); err != nil {
			return fmt.Errorf("error creating collection %s: %w", c, err)
		}
	}
	return nil
}

func (m *MongoDB) createIndexes() error {
	for _, n := range []string{companyTableName, metaTableName} {
		c := m.db.Collection(n)
		var k string
		if n == metaTableName {
			k = keyFieldName
		} else {
			k = idFieldName
		}
		i := []mongo.IndexModel{{Keys: bson.D{{Key: k, Value: 1}}}}
		_, err := c.Indexes().CreateMany(context.Background(), i)
		if err != nil {
			return fmt.Errorf("error creating index for %s in %s: %w", k, n, err)
		}
	}
	return nil
}

// Drop deletes the collectiosn created by `Create`.
func (m *MongoDB) Drop() error {
	for _, n := range []string{companyTableName, metaTableName} {
		slog.Info("Deleting", "collection", n)
		c := m.db.Collection(n)
		if err := c.Drop(context.Background()); err != nil {
			return fmt.Errorf("error deleting collection %s: %w", n, err)
		}
	}
	return nil
}

// CreateCompanies writes a batch of company data to MongoDB
func (m *MongoDB) CreateCompanies(ctx context.Context, batch [][]string) error {
	if m == nil {
		return fmt.Errorf("mongodb connection not initialized")
	}
	coll := m.db.Collection(companyTableName)
	var cs []any // required by MongoDb pkg
	for _, c := range batch {
		if len(c) < 2 {
			return fmt.Errorf("line skipped due to insufficient length: %s", c)
		}
		var r mongoRecord
		r.Id = c[0]
		err := json.Unmarshal([]byte(c[1]), &r.Json)
		if err != nil {
			return fmt.Errorf("error deserializing JSON: %s\nerror: %w", c[1], err)
		}
		cs = append(cs, r)
	}
	if len(cs) == 0 {
		return nil
	}

	_, err := coll.InsertMany(ctx, cs)
	if err != nil {
		return fmt.Errorf("error inserting companies into MongoDB: %w", err)
	}
	return nil
}

// MetaSave inserts if the key doesn't exist, or updates the value if it does.
func (m *MongoDB) MetaSave(k, v string) error {
	c := m.db.Collection(metaTableName)
	if len(k) > 16 {
		return fmt.Errorf("the key can have a maximum of 16 characters")
	}
	f := bson.M{"key": k}
	o := options.UpdateOne().SetUpsert(true) // if it does not exist, creates it
	upd := bson.M{"$set": bson.M{"key": k, "value": v}}
	_, err := c.UpdateOne(context.Background(), f, upd, o)
	if err != nil {
		return fmt.Errorf("error saving %s in the meta collection: %w", k, err)
	}
	return nil
}

// MetaRead reads a key/value pair from the metadata collection.
func (m *MongoDB) MetaRead(k string) (string, error) {
	var result struct {
		Value string `bson:"value"`
	}
	c := m.db.Collection(metaTableName)
	err := c.FindOne(context.Background(), bson.M{"key": k}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", fmt.Errorf("metadata key %s not found", k)
		}
		return "", fmt.Errorf("error looking for metadata key %s: %w", k, err)
	}
	return result.Value, nil
}

// Close terminates the connection to MongoDB.
func (m *MongoDB) Close() {
	if err := m.client.Disconnect(context.Background()); err != nil {
		slog.Error("Error disconnecting from MongoDB", "error", err)
	}
}

// PreLoad runs before starting to load data into the database.
func (m *MongoDB) PreLoad() error {
	return nil
}

// PostLoad runs after loading data into the database. Removes duplicates and
// creates indexes.
func (m *MongoDB) PostLoad() error {
	ctx := context.Background()
	coll := m.db.Collection(companyTableName)
	p := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: fmt.Sprintf("$%s", idFieldName)},
			{Key: "docs", Value: bson.D{{Key: "$push", Value: "$_id"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$match", Value: bson.D{{Key: "count", Value: bson.D{{Key: "$gt", Value: 1}}}}}},
	}
	c, err := coll.Aggregate(ctx, p)
	if err != nil {
		return fmt.Errorf("error executing aggregation: %w", err)
	}
	defer func() {
		if err := c.Close(ctx); err != nil {
			slog.Warn("could not close database connection", "error", err)
		}
	}()
	for c.Next(ctx) {
		var result struct {
			ID   string          `bson:"_id"`
			Docs []bson.ObjectID `bson:"docs"`
		}
		if err := c.Decode(&result); err != nil {
			return fmt.Errorf("error decoding result: %w", err)
		}
		// Keep the first document and remove the others
		if len(result.Docs) > 1 {
			toRemove := result.Docs[1:] // Delete all but the first document
			_, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": toRemove}})
			if err != nil {
				return fmt.Errorf("error removing duplicates: %w", err)
			}
		}
	}
	if err := c.Err(); err != nil {
		return fmt.Errorf("error when iterating through results: %w", err)
	}
	if err := m.createIndexes(); err != nil {
		return fmt.Errorf("error creating indexes: %w", err)
	}
	return nil
}

func (m *MongoDB) GetCompany(ctx context.Context, id string) (string, error) {
	coll := m.db.Collection(companyTableName)
	var r bson.Raw
	err := coll.FindOne(ctx, bson.M{idFieldName: id}).Decode(&r)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", fmt.Errorf("no document found for CNPJ %s", id)
		}
		return "", fmt.Errorf("error querying CNPJ %s: %w", id, err)
	}
	c, err := r.LookupErr("json")
	if err != nil {
		return "", fmt.Errorf("error getting json for company %s: %w", id, err)
	}
	b, err := bson.MarshalExtJSON(c, false, false)
	if err != nil {
		return "", fmt.Errorf("error marshalling json for company %s: %w", id, err)
	}
	return string(b), nil
}

// Search returns paginated results with JSON for companies bases on a search
// query
func (m *MongoDB) Search(ctx context.Context, q *Query) (string, error) {
	coll := m.db.Collection(companyTableName)
	f := bson.M{}
	if len(q.UF) > 0 {
		if len(q.UF) == 1 {
			f["json.uf"] = q.UF[0]
		} else {
			f["json.uf"] = bson.M{"$in": q.UF}
		}
	}
	if len(q.Municipio) > 0 {
		if len(q.Municipio) == 1 {
			f["$or"] = []bson.M{
				{"json.codigo_municipio": q.Municipio[0]},
				{"json.codigo_municipio_ibge": q.Municipio[0]},
			}
		} else {
			f["$or"] = []bson.M{
				{"json.codigo_municipio": bson.M{"$in": q.Municipio}},
				{"json.codigo_municipio_ibge": bson.M{"$in": q.Municipio}},
			}
		}
	}
	if len(q.NaturezaJuridica) > 0 {
		if len(q.NaturezaJuridica) == 1 {
			f["json.codigo_natureza_juridica"] = q.NaturezaJuridica[0]
		} else {
			f["json.codigo_natureza_juridica"] = bson.M{"$in": q.NaturezaJuridica}
		}
	}
	if len(q.CNAEFiscal) > 0 {
		if len(q.CNAEFiscal) == 1 {
			f["json.cnae_fiscal"] = q.CNAEFiscal[0]
		} else {
			f["json.cnae_fiscal"] = bson.M{"$in": q.CNAEFiscal}
		}
	}
	if len(q.CNAE) > 0 {
		if len(q.CNAE) == 1 {
			f["$or"] = []bson.M{
				{"json.cnae_fiscal": q.CNAE[0]},
				{"json.cnaes_secundarios.codigo": bson.M{"$in": q.CNAE}},
			}
		} else {
			f["$or"] = []bson.M{
				{"json.cnae_fiscal": bson.M{"$in": q.CNAE}},
				{"json.cnaes_secundarios.codigo": bson.M{"$in": q.CNAE}},
			}
		}
	}
	if len(q.CNPF) > 0 {
		f["json.qsa.cnpj_cpf_do_socio"] = bson.M{"$in": q.CNPF}
	}
	if q.Cursor != nil {
		id, err := bson.ObjectIDFromHex(*q.Cursor)
		if err != nil {
			return "", fmt.Errorf("error parsing cursor: %w", err)
		}
		f["_id"] = bson.M{"$gt": id}
	}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(q.Limit))
	c, err := coll.Find(ctx, f, opts)
	if err != nil {
		return "", fmt.Errorf("error running query %#v: %w", q, err)
	}
	defer func() {
		if err := c.Close(ctx); err != nil {
			slog.Error("could not close database connection", "error", err)
		}
	}()
	var rs []bson.Raw
	if err := c.All(ctx, &rs); err != nil {
		return "", fmt.Errorf("error decoding results: %w", err)
	}
	var cs []string
	for _, r := range rs {
		c, err := r.LookupErr("json")
		if err != nil {
			return "", fmt.Errorf("error getting json from result: %w", err)
		}
		b, err := bson.MarshalExtJSON(c, false, false)
		if err != nil {
			return "", fmt.Errorf("error marshalling json from result: %w", err)
		}
		cs = append(cs, string(b))
	}
	var cur string
	if len(rs) == int(q.Limit) {
		i := rs[len(rs)-1].Lookup("_id").ObjectID()
		cur = i.Hex()
	}
	return newPage(cs, cur), nil
}

func (m *MongoDB) CreateExtraIndexes(idxs []string) error {
	if err := transform.ValidateIndexes(idxs); err != nil {
		return fmt.Errorf("index name error: %w", err)
	}
	slog.Info("Creating the indexes…")
	c := m.db.Collection(companyTableName)
	var i []mongo.IndexModel
	for _, v := range idxs {
		i = append(i, mongo.IndexModel{
			Keys:    bson.D{{Key: fmt.Sprintf("json.%s", v), Value: 1}},
			Options: options.Index().SetName(fmt.Sprintf("idx_json.%s", v)),
		})
	}
	r, err := c.Indexes().CreateMany(context.Background(), i)
	if err != nil {
		return fmt.Errorf("error creating indexes: %w", err)
	}
	l := "index"
	if len(i) > 1 {
		l = "indexes"
	}
	slog.Info(fmt.Sprintf("%d %s successfully created in the collection %s", len(r), l, companyTableName))
	return nil
}

// AllCompanies returns a paginated list of CNPJ numbers from the database.
func (m *MongoDB) AllCompanies(ctx context.Context, cursor *string, limit uint32) ([]string, *string, error) {
	coll := m.db.Collection(companyTableName)
	f := bson.D{}
	if cursor != nil {
		id, err := bson.ObjectIDFromHex(*cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		f = bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: id}}}}
	}
	opts := options.Find().
		SetProjection(bson.D{{Key: idFieldName, Value: 1}, {Key: "_id", Value: 1}}).
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetLimit(int64(limit))
	c, err := coll.Find(ctx, f, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("error listing CNPJs: %w", err)
	}
	defer func() {
		if err := c.Close(ctx); err != nil {
			slog.Error("could not close database connection", "error", err)
		}
	}()
	var docs []bson.Raw
	if err := c.All(ctx, &docs); err != nil {
		return nil, nil, fmt.Errorf("error reading CNPJs: %w", err)
	}
	ids := make([]string, len(docs))
	for i, doc := range docs {
		id := doc.Lookup(idFieldName)
		if err := id.Validate(); err != nil {
			return nil, nil, fmt.Errorf("error getting ID from document: %w", err)
		}
		ids[i] = id.StringValue()
	}
	if len(docs) < int(limit) {
		return ids, nil, nil
	}
	cur := docs[len(docs)-1].Lookup("_id").ObjectID().Hex()
	return ids, &cur, nil
}

// CreateGraphTable creates the graph collection.
func (m *MongoDB) CreateGraphTable() error {
	ns, err := m.db.ListCollectionNames(context.Background(), bson.M{"name": graphTableName})
	if err != nil {
		return fmt.Errorf("error listing collections: %w", err)
	}
	if len(ns) > 0 {
		return nil
	}
	slog.Info("Creating", "collection", graphTableName)
	coll := m.db.Collection(companyTableName)
	pipeline := mongo.Pipeline{
		{{Key: "$unwind", Value: "$json.qsa"}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "company_id", Value: "$id"},
			{Key: "partner_id", Value: bson.D{{Key: "$cond", Value: bson.D{
				{Key: "if", Value: bson.D{{Key: "$eq", Value: bson.A{"$json.qsa.identificador_de_socio", 1}}}},
				{Key: "then", Value: "$json.qsa.cnpj_cpf_do_socio"},
				{Key: "else", Value: bson.D{{Key: "$concat", Value: bson.A{"$json.qsa.cnpj_cpf_do_socio", "$json.qsa.nome_socio"}}}},
			}}}},
			{Key: "company_name", Value: "$json.razao_social"},
			{Key: "partner_name", Value: "$json.qsa.nome_socio"},
			{Key: "partner_cnpf", Value: "$json.qsa.cnpj_cpf_do_socio"},
			{Key: "partner_type", Value: "$json.qsa.identificador_de_socio"},
		}}},
		{{Key: "$merge", Value: bson.D{
			{Key: "into", Value: graphTableName},
			{Key: "whenMatched", Value: "replace"},
		}}},
	}
	_, err = coll.Aggregate(context.Background(), pipeline)
	if err != nil {
		return fmt.Errorf("error creating graph collection: %w", err)
	}

	g := m.db.Collection(graphTableName)
	idxs := []mongo.IndexModel{
		{Keys: bson.D{{Key: "company_id", Value: 1}}},
		{Keys: bson.D{{Key: "partner_id", Value: 1}}},
	}
	_, err = g.Indexes().CreateMany(context.Background(), idxs)
	if err != nil {
		return fmt.Errorf("error creating indexes for %s: %w", graphTableName, err)
	}
	return nil
}

// GetRelated returns the adjacent nodes of a node in the graph.
func (m *MongoDB) GetRelated(ctx context.Context, id string) ([]GraphEdge, error) {
	coll := m.db.Collection(graphTableName)
	filter := bson.M{
		"$or": []bson.M{
			{"company_id": id},
			{"partner_id": id},
		},
	}
	cur, err := coll.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("error looking for neighbors of %s: %w", id, err)
	}
	defer func() {
		if err := cur.Close(ctx); err != nil {
			slog.Warn("could not close database cursor", "error", err)
		}
	}()

	var edges []GraphEdge
	for cur.Next(ctx) {
		var e GraphEdge
		if err := cur.Decode(&e); err != nil {
			return nil, fmt.Errorf("error decoding neighbor record: %w", err)
		}
		edges = append(edges, e)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("error iterating through neighbors: %w", err)
	}
	return edges, nil
}
