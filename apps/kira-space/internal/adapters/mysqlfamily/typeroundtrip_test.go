package mysqlfamily_test

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func sp(s string) *string { return &s }

// familyTypeRoundTrip is the complete suite's J-GRID-031 row: the MySQL and MariaDB type matrix reads
// back as exact text, and editing `note` leaves every other cell (the TIMESTAMP and the 38-digit
// DECIMAL included) as it was.
func familyTypeRoundTrip(cfg model.ResolvedConnectionConfig) testsupport.Scenario {
	db := testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"))
	return testsupport.TypeRoundTrip(testsupport.TypeRoundTripCase{
		Name:     "mysql family",
		Database: db,
		Table:    testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", "p629_types")),
		Setup: []string{
			"DROP TABLE IF EXISTS p629_types",
			`CREATE TABLE p629_types (id INT PRIMARY KEY, dec_ DECIMAL(38,10) NULL, ubig BIGINT UNSIGNED NULL, bits BIT(8) NULL,
				color ENUM('red','green') NULL, tags SET('a','b','c') NULL, doc JSON NULL, ts TIMESTAMP NULL DEFAULT NULL,
				dt DATETIME NULL, bl BLOB NULL, note VARCHAR(20) NULL)`,
			`INSERT INTO p629_types VALUES
				(1, '12345678901234567890123456.1234567891', 18446744073709551615, b'10100101', 'green', 'a,c', '{"k": [1, 2]}',
				 '2024-03-10 12:34:56', '2024-03-10 12:34:56', X'DEADBEEF', 'x'),
				(2, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL),
				(3, 0, 0, b'0', 'red', '', '{}', '1970-01-02 00:00:01', '1000-01-01 00:00:00', X'', '')`,
			// A zero date is storable only with IGNORE under the default strict mode; it must read, not error.
			`INSERT IGNORE INTO p629_types (id, dt) VALUES (4, '0000-00-00 00:00:00')`,
		},
		Teardown: []string{"DROP TABLE IF EXISTS p629_types"},
		Columns:  []string{"id", "dec_", "ubig", "bits", "color", "tags", "doc", "ts", "dt", "bl", "note"},
		Want: [][]*string{
			{sp("1"), sp("12345678901234567890123456.1234567891"), sp("18446744073709551615"), sp("0xa5"), sp("green"), sp("a,c"), sp(`{"k": [1, 2]}`), sp("2024-03-10 12:34:56"), sp("2024-03-10 12:34:56"), sp("0xdeadbeef"), sp("x")},
			{sp("2"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil},
			{sp("3"), sp("0.0000000000"), sp("0"), sp("0x00"), sp("red"), sp(""), sp("{}"), sp("1970-01-02 00:00:01"), sp("1000-01-01 00:00:00"), sp("0x"), sp("")},
			{sp("4"), nil, nil, nil, nil, nil, nil, nil, sp("0000-00-00 00:00:00"), nil, nil},
		},
		EditID:     "1",
		EditColumn: "note",
		EditValue:  "edited",
	})
}
